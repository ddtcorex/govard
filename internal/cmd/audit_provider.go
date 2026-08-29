package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/audit"
	"govard/internal/engine"
)

// auditLintSelection is the resolved lint backend selection for one audit
// invocation. Exactly one shape is ever populated: the Govard-owned native
// backend, or one explicitly configured external provider. The two are never
// blended and an external provider is never a fallback for the native one, so a
// native failure stays a native failure.
type auditLintSelection struct {
	Provider       string
	External       bool
	ExternalConfig engine.ExternalLintProviderConfig
	Govard         audit.GovardLintOptions
	LintCacheRoot  string
}

// AuditLintSelectionForTest is the observable shape of one resolved selection.
// It exists so command tests can assert which backend would be constructed, and
// with which host resources, without running Docker.
type AuditLintSelectionForTest struct {
	Provider            string
	External            bool
	ExternalImage       string
	ExternalCommand     []string
	ToolchainConfigured bool
	AuthJSON            string
	SSHAgent            string
	AllowSSHAgent       bool
	UID                 int
	GID                 int
	LintCacheRoot       string
}

// effectiveAuditLintProvider applies provider precedence: an explicitly passed
// --lint-provider always wins, then the project's own audit.lint.provider, then
// the flag default. It mirrors how --scope resolves against `audit diff`.
func effectiveAuditLintProvider(options *auditCommandOptions, providerExplicit bool, config *engine.Config) string {
	requested := strings.TrimSpace(options.LintProvider)
	if providerExplicit && requested != "" {
		return requested
	}
	if config != nil {
		if configured := strings.TrimSpace(config.Audit.Lint.Provider); configured != "" {
			return configured
		}
	}
	if requested != "" {
		return requested
	}
	return audit.GovardLintProvider
}

// resolveAuditLintSelection decides which lint backend this invocation uses and
// which host resources it may reach. It performs no Docker work, so an invalid
// selection fails before any image is pulled, built, or run.
func resolveAuditLintSelection(request AuditRunnerRequest) (auditLintSelection, error) {
	provider := strings.TrimSpace(request.LintProvider)
	if provider == "" {
		return auditLintSelection{}, fmt.Errorf("audit lint provider is required (use %q or a configured external provider)", audit.GovardLintProvider)
	}
	govardHome := engine.GovardHomeDir()
	selection := auditLintSelection{Provider: provider, LintCacheRoot: audit.DefaultLintCacheRoot(govardHome)}

	if provider != audit.GovardLintProvider {
		configured, err := auditExternalLintProviderConfig(provider, request.Config)
		if err != nil {
			return auditLintSelection{}, err
		}
		selection.External = true
		selection.ExternalConfig = configured
		return selection, nil
	}

	uid, gid := auditLintContainerUser(request.Config)
	selection.Govard = audit.GovardLintOptions{
		Toolchain: audit.NewToolchainManager(audit.NewExecDockerClient(nil), govardHome),
		AuthJSON:  auditLintComposerAuthPath(),
		// The agent socket is always resolved but never forwarded on its own:
		// the lint backend mounts it only when AllowSSHAgent was opted into.
		SSHAgent:      strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")),
		AllowSSHAgent: request.AllowSSHAgent,
		AllowXdebug:   request.AllowXdebug,
		UID:           uid,
		GID:           gid,
	}
	return selection, nil
}

// auditExternalLintProviderConfig looks up an explicitly selected external
// provider. Nothing is inferred: an unknown name is an error naming the
// provider, never a silent fall back to the native backend.
func auditExternalLintProviderConfig(provider string, config *engine.Config) (engine.ExternalLintProviderConfig, error) {
	if config == nil {
		return engine.ExternalLintProviderConfig{}, fmt.Errorf("audit lint provider %q is not available for a standalone target, which has no project configuration declaring audit.lint.external_providers (use %q)", provider, audit.GovardLintProvider)
	}
	configured, ok := config.Audit.Lint.ExternalProviders[provider]
	if !ok {
		return engine.ExternalLintProviderConfig{}, fmt.Errorf("audit lint provider %q is not a configured audit.lint.external_providers entry (use %q or a configured provider name)", provider, audit.GovardLintProvider)
	}
	return configured, nil
}

// newAuditLintBackend constructs exactly the selected backend. The native
// toolchain manager is never constructed for an external selection, and no
// external provider is ever constructed for a native one.
func newAuditLintBackend(selection auditLintSelection) (audit.LintBackend, error) {
	if selection.External {
		provider, err := audit.NewExternalLintProvider(audit.ExternalLintOptions{ID: selection.Provider, Config: selection.ExternalConfig})
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
	backend, err := audit.NewGovardLintBackend(selection.Govard)
	if err != nil {
		return nil, err
	}
	return backend, nil
}

// newAuditRunner builds the runner for one resolved audit target. A nil backend
// means "construct the selected one"; command tests pass their own backend so
// every other real wiring decision (persisted store, reusable lint cache root,
// provider selection) still runs exactly as it does in production.
//
// Provider selection is skipped entirely when the invocation declares it needs
// no lint backend. Reading a stored session, a stored result, or pruning old
// sessions must not depend on whether a lint provider could be constructed
// right now: an unavailable external provider or a root container identity would
// otherwise make a pure read of a persisted JSON file fail.
func newAuditRunner(request AuditRunnerRequest, backend audit.LintBackend, profilerRuntime audit.ProfilerRuntime) (*audit.Runner, error) {
	govardHome := engine.GovardHomeDir()
	options := audit.RunnerOptions{
		Store:         audit.NewStore(audit.DefaultStoreRoot(govardHome)),
		LintCacheRoot: audit.DefaultLintCacheRoot(govardHome),
		Resources:     audit.Resources{CPU: 8, MemoryMB: 8192},
	}
	if request.ProfilerRuntimeRequired {
		if profilerRuntime == nil {
			var err error
			profilerRuntime, err = newAuditProfilerRuntime(request, defaultAuditProfilerRuntimeDependencies())
			if err != nil {
				return nil, err
			}
		}
		options.ProfilerRuntime = profilerRuntime
	}
	if !request.LintBackendRequired {
		return audit.NewRunner(options), nil
	}
	selection, err := resolveAuditLintSelection(request)
	if err != nil {
		return nil, err
	}
	options.LintCacheRoot = selection.LintCacheRoot
	if backend == nil {
		backend, err = newAuditLintBackend(selection)
		if err != nil {
			return nil, err
		}
	}
	options.LintBackend = backend
	return audit.NewRunner(options), nil
}

// auditLintContainerUser resolves the host identity the lint container runs as.
// A loaded project configuration already carries the normalized, Windows-safe
// host UID/GID for container user mapping, so it is reused verbatim; only a
// standalone target (which has no project configuration) repeats the same
// fallback here.
func auditLintContainerUser(config *engine.Config) (int, int) {
	if config != nil && config.Stack.UserID != 0 && config.Stack.GroupID != 0 {
		return config.Stack.UserID, config.Stack.GroupID
	}
	uid := os.Getuid()
	if uid < 0 {
		uid = 1000
	}
	gid := os.Getgid()
	if gid < 0 {
		gid = 1000
	}
	return uid, gid
}

// auditLintComposerAuthPath detects the host Composer credentials the lint
// container may need for private packages, using the same convention as the
// rest of Govard. An absent file is not an error: the lint backend simply
// mounts nothing.
func auditLintComposerAuthPath() string {
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	composerConfigDir := filepath.Join(home, ".composer")
	if info, err := os.Stat(composerConfigDir); err != nil || !info.IsDir() {
		return ""
	}
	authPath := filepath.Join(composerConfigDir, "auth.json")
	if info, err := os.Stat(authPath); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return authPath
}

// ResolveAuditLintSelectionForTest exposes provider selection to command tests.
func ResolveAuditLintSelectionForTest(request AuditRunnerRequest) (AuditLintSelectionForTest, error) {
	selection, err := resolveAuditLintSelection(request)
	if err != nil {
		return AuditLintSelectionForTest{}, err
	}
	return AuditLintSelectionForTest{
		Provider:            selection.Provider,
		External:            selection.External,
		ExternalImage:       selection.ExternalConfig.Image,
		ExternalCommand:     append([]string(nil), selection.ExternalConfig.Command...),
		ToolchainConfigured: selection.Govard.Toolchain != nil,
		AuthJSON:            selection.Govard.AuthJSON,
		SSHAgent:            selection.Govard.SSHAgent,
		AllowSSHAgent:       selection.Govard.AllowSSHAgent,
		UID:                 selection.Govard.UID,
		GID:                 selection.Govard.GID,
		LintCacheRoot:       selection.LintCacheRoot,
	}, nil
}

// NewAuditLintBackendForTest resolves and constructs the selected lint backend
// so command tests can assert which implementation a selection produces.
func NewAuditLintBackendForTest(request AuditRunnerRequest) (audit.LintBackend, error) {
	selection, err := resolveAuditLintSelection(request)
	if err != nil {
		return nil, err
	}
	return newAuditLintBackend(selection)
}
