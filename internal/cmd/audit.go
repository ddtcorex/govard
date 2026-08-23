package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"govard/internal/audit"
	"govard/internal/engine"
	"govard/internal/frameworks/types"

	"github.com/spf13/cobra"
)

type auditCommandOptions struct {
	Scope             string
	BaseRef           string
	Checks            []string
	Format            string
	SessionID         string
	RunID             string
	LintProvider      string
	LintJobs          int
	NoLintResultCache bool
	AllowLintSSHAgent bool
	OlderThan         time.Duration
	TargetMode        string
	PHPVersions       []string
	URL               string
}

// AuditRunnerRequest is the resolved context a runner factory needs to build a
// runner with the right lint backend. Provider selection, the container user
// identity, and the reusable cache root all derive from it, so it carries the
// resolved target and its loaded project configuration rather than a bare path.
type AuditRunnerRequest struct {
	ProjectRoot string
	Definition  types.FrameworkDefinition
	Target      types.AuditTarget
	// Config is the loaded project configuration; it is nil for a standalone
	// target, which has no project to load one from.
	Config *engine.Config
	// LintBackendRequired reports whether this invocation actually executes
	// lint. It is false for the read-only commands (status, result, cleanup),
	// which only read persisted sessions and must never depend on a lint
	// provider being constructible.
	LintBackendRequired bool
	// ProfilerRuntimeRequired reports whether this invocation executes a
	// framework-owned runtime profiler.
	ProfilerRuntimeRequired bool
	// LintProvider is the effective provider name after flag/config precedence.
	// It stays empty when LintBackendRequired is false, since no provider is
	// selected for a read-only command.
	LintProvider string
	// AllowSSHAgent mirrors --allow-lint-ssh-agent. Agent forwarding is never
	// inferred from the host environment alone.
	AllowSSHAgent bool
}

// auditPreparation states what one audit invocation actually needs, so the
// read-only commands neither probe a running PHP runtime nor select a lint
// provider. Both facts are independent: rerun executes lint but reloads its PHP
// matrix from the persisted session instead of resolving policy again.
type auditPreparation struct {
	ResolvePHPPolicy        bool
	LintBackendRequired     bool
	ProfilerRuntimeRequired bool
	RequireProfilerURL      bool
}

type auditCommandDependencies struct {
	runnerFactory    func(AuditRunnerRequest) (*audit.Runner, error)
	toolchainFactory func() (*audit.ToolchainManager, error)
	runtimePHPProbe  runtimePHPProbe
}

// AuditDependenciesForTest replaces lint execution and toolchain management in
// command tests.
type AuditDependenciesForTest struct {
	// RunnerFactory replaces runner construction entirely.
	RunnerFactory func(AuditRunnerRequest) (*audit.Runner, error)
	// LintBackend replaces only the constructed lint backend, leaving real
	// provider selection, store, and reusable lint cache wiring in place.
	LintBackend audit.LintBackend
	// ToolchainFactory replaces the Docker-backed toolchain manager the
	// project-independent toolchain commands use.
	ToolchainFactory func() (*audit.ToolchainManager, error)
	RuntimePHPProbe  func(context.Context, types.AuditTarget, engine.Config) (version string, running bool, err error)
	ProfilerRuntime  audit.ProfilerRuntime
}

var auditDependencyState struct {
	sync.RWMutex
	override *AuditDependenciesForTest
}

// SetAuditDependenciesForTest installs a test-only runner factory and returns
// a restoration function.
func SetAuditDependenciesForTest(dependencies AuditDependenciesForTest) func() {
	auditDependencyState.Lock()
	previous := auditDependencyState.override
	auditDependencyState.override = &dependencies
	auditDependencyState.Unlock()
	return func() {
		auditDependencyState.Lock()
		auditDependencyState.override = previous
		auditDependencyState.Unlock()
	}
}

// ResetAuditCommandForTest gives each command test fresh command-local flags.
func ResetAuditCommandForTest() {
	for _, command := range rootCmd.Commands() {
		if command.Name() == "audit" {
			rootCmd.RemoveCommand(command)
		}
	}
	rootCmd.AddCommand(newAuditCommand(defaultAuditCommandDependencies()))
}

func defaultAuditCommandDependencies() auditCommandDependencies {
	return auditCommandDependencies{
		runnerFactory: func(request AuditRunnerRequest) (*audit.Runner, error) {
			return newAuditRunner(request, nil, nil)
		},
		toolchainFactory: func() (*audit.ToolchainManager, error) {
			return audit.NewToolchainManager(audit.NewExecDockerClient(nil), engine.GovardHomeDir()), nil
		},
		runtimePHPProbe: probeRuntimePHP,
	}
}

func currentAuditDependencies(defaults auditCommandDependencies) auditCommandDependencies {
	auditDependencyState.RLock()
	override := auditDependencyState.override
	auditDependencyState.RUnlock()
	if override == nil {
		return defaults
	}
	if override.RunnerFactory != nil {
		defaults.runnerFactory = override.RunnerFactory
	}
	if override.LintBackend != nil {
		backend := override.LintBackend
		defaults.runnerFactory = func(request AuditRunnerRequest) (*audit.Runner, error) {
			return newAuditRunner(request, backend, override.ProfilerRuntime)
		}
	} else if override.ProfilerRuntime != nil {
		profilerRuntime := override.ProfilerRuntime
		defaults.runnerFactory = func(request AuditRunnerRequest) (*audit.Runner, error) {
			return newAuditRunner(request, nil, profilerRuntime)
		}
	}
	if override.ToolchainFactory != nil {
		defaults.toolchainFactory = override.ToolchainFactory
	}
	// A substituted lint backend also means no real container exists to probe,
	// so the runtime PHP probe is stubbed unless the test supplies its own.
	if override.RunnerFactory != nil || override.LintBackend != nil {
		defaults.runtimePHPProbe = func(context.Context, types.AuditTarget, engine.Config) (string, bool, error) {
			return "", false, nil
		}
	}
	if override.RuntimePHPProbe != nil {
		defaults.runtimePHPProbe = runtimePHPProbe(override.RuntimePHPProbe)
	}
	return defaults
}

func newAuditCommand(dependencies auditCommandDependencies) *cobra.Command {
	options := &auditCommandOptions{Scope: string(audit.ScopeProject), Checks: []string{"lint"}, Format: "text", LintProvider: audit.GovardLintProvider, LintJobs: 2}
	command := &cobra.Command{
		Use:   "audit",
		Short: "Run and inspect persistent project audits",
	}
	command.PersistentFlags().StringVar(&options.Scope, "scope", string(audit.ScopeProject), "Audit scope (project or diff)")
	command.PersistentFlags().StringVar(&options.BaseRef, "base", "", "Base ref required for diff scope")
	command.PersistentFlags().StringSliceVar(&options.Checks, "checks", []string{"lint"}, "Checks to run (lint or profiler)")
	command.PersistentFlags().StringVar(&options.Format, "format", "text", "Output format (text or json)")
	command.PersistentFlags().StringVar(&options.SessionID, "session", "", "Explicit audit session ID")
	command.PersistentFlags().StringVar(&options.RunID, "run", "", "Explicit audit run ID")
	command.PersistentFlags().StringVar(&options.LintProvider, "lint-provider", audit.GovardLintProvider, "Lint provider: govard, or an audit.lint.external_providers name from the project config")
	command.PersistentFlags().IntVar(&options.LintJobs, "lint-jobs", 2, "Lint worker count")
	command.PersistentFlags().BoolVar(&options.NoLintResultCache, "no-lint-result-cache", false, "Ignore reusable lint analyzer state for this run (the Composer download cache is kept)")
	command.PersistentFlags().BoolVar(&options.AllowLintSSHAgent, "allow-lint-ssh-agent", false, "Forward SSH_AUTH_SOCK into the lint container for private Composer dependencies")
	command.PersistentFlags().DurationVar(&options.OlderThan, "older-than", 0, "Remove sessions older than this duration")
	command.PersistentFlags().StringVar(&options.TargetMode, "mode", "auto", "Audit target mode (auto, project, or standalone)")
	command.PersistentFlags().StringSliceVar(&options.PHPVersions, "php", nil, "PHP versions; standalone only unless matching active project PHP")
	command.PersistentFlags().StringVar(&options.URL, "url", "", "Absolute HTTP(S) URL captured by runtime audit checks")

	command.AddCommand(
		newAuditRunCommand(options, dependencies, false),
		newAuditRunCommand(options, dependencies, true),
		newAuditRerunCommand(options, dependencies),
		newAuditStatusCommand(options, dependencies),
		newAuditResultCommand(options, dependencies),
		newAuditCleanupCommand(options, dependencies),
		newAuditToolchainCommand(options, dependencies),
	)
	return command
}

func newAuditRunCommand(options *auditCommandOptions, dependencies auditCommandDependencies, forceDiff bool) *cobra.Command {
	use := "run"
	short := "Run a project audit"
	if forceDiff {
		use = "diff"
		short = "Record a diff audit and run its exact checks"
	}
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateAuditCommandOptions(options); err != nil {
			return err
		}
		scope, err := auditScope(options.Scope, forceDiff, cmd.Flags().Changed("scope"))
		if err != nil {
			return err
		}
		if scope == audit.ScopeDiff && strings.TrimSpace(options.BaseRef) == "" {
			return errors.New("audit diff requires --base")
		}
		lintRequested := auditChecksInclude(options.Checks, "lint")
		profilerRequested := auditChecksInclude(options.Checks, "profiler")
		runner, resolvedTarget, err := prepareAudit(cmd, options, dependencies, auditPreparation{
			ResolvePHPPolicy:        lintRequested,
			LintBackendRequired:     lintRequested,
			ProfilerRuntimeRequired: profilerRequested,
			RequireProfilerURL:      profilerRequested,
		})
		if err != nil {
			return err
		}
		if err := validateAuditOptions(options, resolvedTarget.Definition); err != nil {
			return err
		}
		lintProfile := types.AuditLintProfile{}
		if resolvedTarget.Definition.AuditLint != nil {
			lintProfile = *resolvedTarget.Definition.AuditLint
		}
		result, err := runner.Run(cmd.Context(), audit.RunRequest{
			ProjectRoot:         auditTargetRoot(resolvedTarget.Target),
			ProjectID:           resolvedTarget.ProjectID,
			Scope:               scope,
			BaseRef:             options.BaseRef,
			Checks:              options.Checks,
			LintJobs:            options.LintJobs,
			Environment:         auditTargetEnvironment(resolvedTarget),
			Source:              resolvedTarget.Source,
			LintProfile:         lintProfile,
			Target:              resolvedTarget.Target,
			ProfilerURL:         options.URL,
			SelectedPHPVersions: resolvedTarget.PHPVersions,
			MatrixComplete:      resolvedTarget.MatrixComplete,
			BypassResultCache:   options.NoLintResultCache,
		})
		// Render the summary before reporting the outcome, so a failed run
		// still prints everything the operator needs alongside the failure.
		// A run without a persisted identity produced nothing worth showing;
		// its error carries the whole story.
		var renderErr error
		if result.RunID != "" {
			renderErr = writeAuditValue(cmd, options.Format, result)
		}
		if err != nil {
			return auditRunExitError{cause: err}
		}
		if renderErr != nil {
			return renderErr
		}
		return auditRunOutcome(result)
	}}
}

func newAuditRerunCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{Use: "rerun", Short: "Rerun an explicit audit session", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateAuditCommandOptions(options); err != nil {
			return err
		}
		if strings.TrimSpace(options.SessionID) == "" {
			return errors.New("audit rerun requires --session")
		}
		// Without an explicit --checks the rerun repeats the latest run's
		// selection; peek it straight from the persisted session store so no
		// lint backend or profiler runtime is constructed for the lookup.
		effectiveChecks := options.Checks
		if !cmd.Flags().Changed("checks") {
			mode := types.AuditTargetMode(strings.TrimSpace(options.TargetMode))
			if mode == "" {
				mode = types.AuditTargetAuto
			}
			resolvedDeps := currentAuditDependencies(dependencies)
			peekTarget, err := resolveAuditTarget(cmd.Context(), commandStartDirectory(), mode, options.PHPVersions, resolvedDeps.runtimePHPProbe, false)
			if err != nil {
				return err
			}
			peekRunner := audit.NewRunner(audit.RunnerOptions{Store: audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir()))})
			effectiveChecks, err = peekRunner.LatestRunChecks(cmd.Context(), peekTarget.ProjectID, options.SessionID)
			if err != nil {
				return err
			}
		}
		lintRequested := auditChecksInclude(effectiveChecks, "lint")
		profilerRequested := auditChecksInclude(effectiveChecks, "profiler")
		runner, resolvedTarget, err := prepareAudit(cmd, options, dependencies, auditPreparation{
			LintBackendRequired:     lintRequested,
			ProfilerRuntimeRequired: profilerRequested,
		})
		if err != nil {
			return err
		}
		if err := validateAuditOptions(options, resolvedTarget.Definition); err != nil {
			return err
		}
		result, err := runner.Rerun(cmd.Context(), options.SessionID, resolvedTarget.ProjectID, effectiveChecks)
		var renderErr error
		if result.RunID != "" {
			renderErr = writeAuditValue(cmd, options.Format, result)
		}
		if err != nil {
			return auditRunExitError{cause: err}
		}
		if renderErr != nil {
			return renderErr
		}
		return auditRunOutcome(result)
	}}
}

func newAuditStatusCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show an explicit audit session", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateAuditCommandOptions(options); err != nil {
			return err
		}
		if strings.TrimSpace(options.SessionID) == "" {
			return errors.New("audit status requires --session")
		}
		runner, target, err := prepareAudit(cmd, options, dependencies, auditPreparation{})
		if err != nil {
			return err
		}
		if target.Definition.AuditLint == nil {
			return fmt.Errorf("framework %q does not support lint audit", target.Definition.Name)
		}
		manifest, err := runner.Status(target.ProjectID, options.SessionID)
		if err != nil {
			return err
		}
		return writeAuditValue(cmd, options.Format, manifest)
	}}
}

func newAuditResultCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{Use: "result", Short: "Show an explicit audit run result", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateAuditCommandOptions(options); err != nil {
			return err
		}
		if strings.TrimSpace(options.SessionID) == "" || strings.TrimSpace(options.RunID) == "" {
			return errors.New("audit result requires --session and --run")
		}
		runner, target, err := prepareAudit(cmd, options, dependencies, auditPreparation{})
		if err != nil {
			return err
		}
		if target.Definition.AuditLint == nil {
			return fmt.Errorf("framework %q does not support lint audit", target.Definition.Name)
		}
		result, err := runner.Result(target.ProjectID, options.SessionID, options.RunID)
		if err != nil {
			return err
		}
		return writeAuditValue(cmd, options.Format, result)
	}}
}

func newAuditCleanupCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{Use: "cleanup", Short: "Remove old persisted audit sessions", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateAuditCommandOptions(options); err != nil {
			return err
		}
		if !cmd.Flags().Changed("older-than") || options.OlderThan <= 0 {
			return errors.New("audit cleanup requires a positive --older-than duration")
		}
		runner, target, err := prepareAudit(cmd, options, dependencies, auditPreparation{})
		if err != nil {
			return err
		}
		if target.Definition.AuditLint == nil {
			return fmt.Errorf("framework %q does not support lint audit", target.Definition.Name)
		}
		removed, err := runner.CleanupOlderThan(target.ProjectID, time.Now().Add(-options.OlderThan))
		if err != nil {
			return err
		}
		return writeAuditValue(cmd, options.Format, map[string]any{"removed_sessions": removed})
	}}
}

func prepareAudit(cmd *cobra.Command, options *auditCommandOptions, dependencies auditCommandDependencies, preparation auditPreparation) (*audit.Runner, resolvedAuditTarget, error) {
	mode := types.AuditTargetMode(strings.TrimSpace(options.TargetMode))
	if mode == "" {
		mode = types.AuditTargetAuto
	}
	resolved := currentAuditDependencies(dependencies)
	target, err := resolveAuditTarget(cmd.Context(), commandStartDirectory(), mode, options.PHPVersions, resolved.runtimePHPProbe, preparation.ResolvePHPPolicy)
	if err != nil {
		return nil, resolvedAuditTarget{}, err
	}
	request := AuditRunnerRequest{
		ProjectRoot:             auditTargetRoot(target.Target),
		Definition:              target.Definition,
		Target:                  target.Target,
		Config:                  target.Config,
		LintBackendRequired:     preparation.LintBackendRequired,
		ProfilerRuntimeRequired: preparation.ProfilerRuntimeRequired,
	}
	if preparation.ProfilerRuntimeRequired {
		if target.Definition.AuditProfiler == nil {
			return nil, resolvedAuditTarget{}, fmt.Errorf("framework %q does not support profiler audit", target.Definition.Name)
		}
		if target.Config == nil || target.Target.Mode == types.AuditTargetStandalone {
			return nil, resolvedAuditTarget{}, errors.New("audit profiler does not support standalone targets; run it from a Govard project")
		}
		if target.Target.Mode != types.AuditTargetProject {
			return nil, resolvedAuditTarget{}, fmt.Errorf("audit profiler requires a project target, got %q", target.Target.Mode)
		}
		if preparation.RequireProfilerURL {
			if err := audit.ValidateProfilerURL(options.URL); err != nil {
				return nil, resolvedAuditTarget{}, err
			}
		}
	}
	// Provider precedence is only resolved for an invocation that actually runs
	// lint; a read-only command must not select a provider at all.
	if preparation.LintBackendRequired {
		request.LintProvider = effectiveAuditLintProvider(options, cmd.Flags().Changed("lint-provider"), target.Config)
		request.AllowSSHAgent = options.AllowLintSSHAgent
	}
	runner, err := resolved.runnerFactory(request)
	if err != nil {
		return nil, resolvedAuditTarget{}, err
	}
	return runner, target, nil
}

func commandStartDirectory() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return workingDirectory
}

func auditScope(value string, forceDiff, scopeExplicit bool) (audit.Scope, error) {
	scope := audit.Scope(strings.TrimSpace(value))
	if scope != audit.ScopeProject && scope != audit.ScopeDiff {
		return "", fmt.Errorf("unsupported audit scope %q", value)
	}
	if forceDiff {
		if scopeExplicit && scope != audit.ScopeDiff {
			return "", errors.New("audit diff conflicts with --scope project; use --scope diff or omit it")
		}
		return audit.ScopeDiff, nil
	}
	return scope, nil
}

func validateAuditOptions(options *auditCommandOptions, definition types.FrameworkDefinition) error {
	if err := validateAuditCommandOptions(options); err != nil {
		return err
	}
	if auditChecksInclude(options.Checks, "lint") {
		if definition.AuditLint == nil {
			return fmt.Errorf("framework %q does not support lint audit", definition.Name)
		}
		if options.LintJobs < 1 || options.LintJobs > len(definition.AuditLint.ProjectPHPVersions) {
			return fmt.Errorf("lint jobs must be between 1 and %d", len(definition.AuditLint.ProjectPHPVersions))
		}
	}
	return nil
}

func auditChecksInclude(checks []string, candidate string) bool {
	for _, check := range checks {
		if strings.TrimSpace(check) == candidate {
			return true
		}
	}
	return false
}

func validateAuditCommandOptions(options *auditCommandOptions) error {
	if _, err := auditScope(options.Scope, false, false); err != nil {
		return err
	}
	if _, err := audit.NormalizeChecks(options.Checks); err != nil {
		return err
	}
	if err := validateAuditProviderOption(options); err != nil {
		return err
	}
	return validateAuditOutputOptions(options)
}

// validateAuditProviderOption only rejects a syntactically unusable provider
// name. Whether a name is actually available is decided against the project's
// configured external providers when the backend is selected, since only that
// layer knows them.
func validateAuditProviderOption(options *auditCommandOptions) error {
	if strings.TrimSpace(options.LintProvider) == "" {
		return fmt.Errorf("audit lint provider must not be empty (use %q or a configured external provider)", audit.GovardLintProvider)
	}
	return nil
}

func validateAuditOutputOptions(options *auditCommandOptions) error {
	if options.Format != "text" && options.Format != "json" {
		return fmt.Errorf("unsupported audit format %q (use text or json)", options.Format)
	}
	return nil
}

func auditEnvironment(config engine.Config) audit.EnvironmentFingerprint {
	return audit.EnvironmentFingerprint{
		Framework:        config.Framework,
		FrameworkVersion: config.FrameworkVersion,
		GovardVersion:    Version,
		WebServer:        config.Stack.Services.WebServer,
	}
}

func auditTargetRoot(target types.AuditTarget) string {
	if target.ProjectRoot != "" {
		return target.ProjectRoot
	}
	return target.TargetPath
}

func auditTargetEnvironment(target resolvedAuditTarget) audit.EnvironmentFingerprint {
	if target.Config != nil {
		return auditEnvironment(*target.Config)
	}
	return audit.EnvironmentFingerprint{Framework: target.Definition.Name, GovardVersion: Version}
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func auditManifestDigest(root string) string {
	hash := sha256.New()
	for _, name := range []string{".govard.yml", "composer.json", "composer.lock"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		_, _ = io.WriteString(hash, name+"\n")
		_, _ = hash.Write(content)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func writeAuditJSON(writer io.Writer, value any) error {
	return json.NewEncoder(writer).Encode(value)
}

func writeAuditValue(cmd *cobra.Command, format string, value any) error {
	if format == "json" {
		return writeAuditJSON(cmd.OutOrStdout(), value)
	}
	return writeAuditText(cmd.OutOrStdout(), value)
}

// auditRunExitError wraps a completed-but-unsuccessful run so the command can
// render its full summary first and only then fail the process.
type auditRunExitError struct {
	cause error
}

func (e auditRunExitError) Error() string { return e.cause.Error() }
func (e auditRunExitError) Unwrap() error { return e.cause }
