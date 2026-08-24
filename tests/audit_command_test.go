package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks/types"

	"github.com/spf13/pflag"
)

type commandLintBackend struct {
	runs     int
	requests []audit.LintRequest
}

func (backend *commandLintBackend) Name() string { return "ci" }

func (backend *commandLintBackend) Run(_ context.Context, request audit.LintRequest) (audit.LintReport, error) {
	backend.runs++
	backend.requests = append(backend.requests, request)
	return passingLintReport(request.ProjectID), nil
}

func TestAuditRunUsesConfiguredProjectPHPVersion(t *testing.T) {
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.3")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	if _, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	if len(backend.requests) != 1 {
		t.Fatalf("lint requests = %d, want 1", len(backend.requests))
	}
	request := backend.requests[0]
	if got, want := request.SelectedPHPVersions, []string{"8.3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected PHP versions = %#v, want %#v", got, want)
	}
	if !request.MatrixComplete {
		t.Fatal("single configured project PHP version was not marked matrix-complete")
	}
}

func TestAuditRunJSONReturnsSessionAndRunIDs(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("JSON output has terminal decoration or is invalid: %v; output=%q", err, output)
	}
	if result.SessionID == "" || result.RunID == "" {
		t.Fatalf("result IDs = %q/%q", result.SessionID, result.RunID)
	}
}

func TestAuditDiffRequiresBase(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	_, err := executeAuditCommand(t, project, []string{"audit", "diff"})
	if err == nil || !strings.Contains(err.Error(), "--base") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuditDiffRejectsInvalidOrConflictingScope(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	for _, scope := range []string{"invalid", "project"} {
		_, err := executeAuditCommand(t, project, []string{"audit", "diff", "--base", "origin/master", "--scope", scope})
		if err == nil || !strings.Contains(err.Error(), "scope") {
			t.Fatalf("scope %q error = %v", scope, err)
		}
	}
}

func TestAuditRerunRequiresExplicitSession(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	_, err := executeAuditCommand(t, project, []string{"audit", "rerun"})
	if err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("error = %v", err)
	}
}

// A rerun without --checks must repeat the checks of the latest run in the
// session instead of silently falling back to the [lint] default, which fails
// on sessions that never carried lint settings.
func TestAuditRerunWithoutChecksRepeatsLatestRunChecks(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	runtime := &fakeProfilerRuntime{csv: []byte("type,timer\nfoo,1\n")}
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{ProfilerRuntime: runtime})
	t.Cleanup(restore)

	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--checks", "profiler", "--url", "https://shop.test/", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var first audit.RunResult
	if err := json.Unmarshal(output, &first); err != nil {
		t.Fatal(err)
	}
	if first.Status != audit.StatusPassed {
		t.Fatalf("first run status = %q, want passed", first.Status)
	}

	if _, err := executeAuditCommand(t, project, []string{"audit", "rerun", "--session", first.SessionID, "--format", "json"}); err != nil {
		t.Fatalf("rerun without --checks: %v", err)
	}
	if len(runtime.activateRequests) != 2 {
		t.Fatalf("profiler activations = %d, want 2 (rerun must repeat the profiler check)", len(runtime.activateRequests))
	}
}

func TestAuditStatusDoesNotExecuteBackend(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	backend.runs = 0
	if _, err := executeAuditCommand(t, project, []string{"audit", "status", "--session", result.SessionID, "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	if backend.runs != 0 {
		t.Fatalf("status executed lint backend %d times", backend.runs)
	}
}

func TestAuditStatusRejectsUnsupportedFormat(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "status", "--session", "session-a", "--format", "yaml"})
	if err == nil || !strings.Contains(err.Error(), "unsupported audit format") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuditStatusRejectsUnsupportedChecks(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "status", "--session", "session-a", "--checks", "browser"})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuditStatusAcceptsProfilerChecks(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "status", "--session", "session-a", "--checks", "profiler"})
	if err == nil {
		t.Fatal("audit status unexpectedly succeeded for a missing session")
	}
	if strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error = %v, want profiler to pass command validation", err)
	}
}

func TestAuditProfilerRunRequiresExplicitURLBeforeRunnerConstruction(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	var factoryCalls int
	installAuditRunnerFactory(t, func(_ cmd.AuditRunnerRequest) (*audit.Runner, error) {
		factoryCalls++
		return nil, nil
	})

	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--checks", "profiler"})
	if err == nil || !strings.Contains(err.Error(), "--url") {
		t.Fatalf("error = %v, want explicit profiler --url requirement", err)
	}
	if factoryCalls != 0 {
		t.Fatalf("runner factory calls = %d, want none before URL validation", factoryCalls)
	}
}

func TestAuditProfilerCommandPersistsURLForRerun(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	runtime := &fakeProfilerRuntime{csv: []byte("type,timer\nfoo,1\n")}
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{ProfilerRuntime: runtime})
	t.Cleanup(restore)
	targetURL := "https://audit-shop.test/category.html?product_list_limit=48&color=red%20blue"

	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--checks", "profiler", "--url", targetURL, "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var first audit.RunResult
	if err := json.Unmarshal(output, &first); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAuditCommand(t, project, []string{"audit", "rerun", "--session", first.SessionID, "--checks", "profiler", "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.activateRequests) != 2 {
		t.Fatalf("activate requests = %d, want 2", len(runtime.activateRequests))
	}
	for index, request := range runtime.activateRequests {
		if request.URL != targetURL {
			t.Fatalf("activate request %d URL = %q, want %q", index, request.URL, targetURL)
		}
	}
}

func TestAuditProfilerCommandRejectsStandaloneTarget(t *testing.T) {
	module := standaloneAuditModule(t, "vendor/package")
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{ProfilerRuntime: &fakeProfilerRuntime{}})
	t.Cleanup(restore)

	_, err := executeAuditCommand(t, module, []string{"audit", "run", "--mode", "standalone", "--checks", "profiler", "--url", "https://shop.test/"})
	if err == nil || !strings.Contains(err.Error(), "standalone") {
		t.Fatalf("error = %v, want standalone profiler rejection", err)
	}
}

func TestAuditCleanupRejectsInvalidDuration(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	_, err := executeAuditCommand(t, project, []string{"audit", "cleanup", "--older-than", "yesterday"})
	if err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuditCleanupUsesInjectedRunnerStore(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &commandLintBackend{}
	store := audit.NewStore(filepath.Join(t.TempDir(), "injected-audit"))
	installAuditRunnerFactory(t, func(_ cmd.AuditRunnerRequest) (*audit.Runner, error) {
		return audit.NewRunner(audit.RunnerOptions{
			Store:       store,
			LintBackend: backend,
			Resources:   audit.Resources{CPU: 4, MemoryMB: 4096},
		}), nil
	})
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	output, err = executeAuditCommand(t, project, []string{"audit", "cleanup", "--older-than", "1ns", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var cleanup struct {
		RemovedSessions []string `json:"removed_sessions"`
	}
	if err := json.Unmarshal(output, &cleanup); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.RemovedSessions) != 1 || cleanup.RemovedSessions[0] != result.SessionID {
		t.Fatalf("removed sessions = %#v, want %q", cleanup.RemovedSessions, result.SessionID)
	}
}

func TestAuditRejectsFrameworkWithoutAuditTargetCapability(t *testing.T) {
	project := auditCommandProject(t, "laravel")
	_, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil || !strings.Contains(err.Error(), "no framework can resolve audit target") {
		t.Fatalf("error = %v, want audit target capability failure", err)
	}
}

func TestAuditProviderSelectionDefaultsToNativeGovardBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("SSH_AUTH_SOCK", "/run/user/1000/keyring/ssh")
	request := auditRunnerRequestForProvider(t, "govard", auditProjectConfigWithExternalProvider("vendor-lint"))

	selection, err := cmd.ResolveAuditLintSelectionForTest(request)
	if err != nil {
		t.Fatalf("ResolveAuditLintSelectionForTest returned error: %v", err)
	}
	if selection.Provider != "govard" || selection.External {
		t.Fatalf("selection = %#v, want the native govard provider", selection)
	}
	if !selection.ToolchainConfigured {
		t.Fatalf("selection = %#v, want a configured toolchain manager", selection)
	}
	if selection.UID != 4242 || selection.GID != 4243 {
		t.Fatalf("container user = %d:%d, want the normalized host identity 4242:4243", selection.UID, selection.GID)
	}
	if selection.SSHAgent != "/run/user/1000/keyring/ssh" {
		t.Fatalf("ssh agent = %q, want the resolved SSH_AUTH_SOCK", selection.SSHAgent)
	}
	if selection.AllowSSHAgent {
		t.Fatal("ssh agent forwarding was allowed without --allow-lint-ssh-agent")
	}
	if want := audit.DefaultLintCacheRoot(home); selection.LintCacheRoot != want {
		t.Fatalf("lint cache root = %q, want %q", selection.LintCacheRoot, want)
	}

	backend, err := cmd.NewAuditLintBackendForTest(request)
	if err != nil {
		t.Fatalf("NewAuditLintBackendForTest returned error: %v", err)
	}
	if _, ok := backend.(*audit.GovardLintBackend); !ok {
		t.Fatalf("backend = %T, want *audit.GovardLintBackend", backend)
	}
	if backend.Name() != "govard" {
		t.Fatalf("backend name = %q, want %q", backend.Name(), "govard")
	}
}

func TestAuditProviderSelectionUsesExplicitlyConfiguredExternalProvider(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	request := auditRunnerRequestForProvider(t, "vendor-lint", auditProjectConfigWithExternalProvider("vendor-lint"))

	selection, err := cmd.ResolveAuditLintSelectionForTest(request)
	if err != nil {
		t.Fatalf("ResolveAuditLintSelectionForTest returned error: %v", err)
	}
	if !selection.External || selection.Provider != "vendor-lint" {
		t.Fatalf("selection = %#v, want the configured external provider", selection)
	}
	if selection.ToolchainConfigured {
		t.Fatal("an external provider selection constructed a native lint toolchain manager")
	}
	if selection.ExternalImage != "registry.example.test/vendor-lint:1.0" {
		t.Fatalf("external image = %q, want the configured image", selection.ExternalImage)
	}
	if got, want := selection.ExternalCommand, []string{"lint"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("external command = %#v, want %#v", got, want)
	}

	backend, err := cmd.NewAuditLintBackendForTest(request)
	if err != nil {
		t.Fatalf("NewAuditLintBackendForTest returned error: %v", err)
	}
	if _, ok := backend.(*audit.ExternalLintProvider); !ok {
		t.Fatalf("backend = %T, want *audit.ExternalLintProvider", backend)
	}
	if backend.Name() != "vendor-lint" {
		t.Fatalf("backend name = %q, want %q", backend.Name(), "vendor-lint")
	}
}

func TestAuditProviderSelectionRejectsUnknownExternalProvider(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	for name, config := range map[string]*engine.Config{
		"configured project": auditProjectConfigWithExternalProvider("vendor-lint"),
		"standalone target":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			request := auditRunnerRequestForProvider(t, "missing-lint", config)
			_, err := cmd.ResolveAuditLintSelectionForTest(request)
			if err == nil {
				t.Fatal("unknown external provider was accepted")
			}
			if !strings.Contains(err.Error(), "missing-lint") {
				t.Fatalf("error = %v, want it to name the unknown provider", err)
			}
			if _, err := cmd.NewAuditLintBackendForTest(request); err == nil {
				t.Fatal("unknown external provider produced a lint backend")
			}
		})
	}
}

func TestAuditProviderSelectionNeverFallsBackToAnExternalProvider(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	// The native toolchain degrades from the pinned official image to the
	// embedded local build on its own (see the toolchain manager tests). A
	// configured external provider must never be reached by that path, and a
	// native construction failure must surface as a failure rather than
	// silently selecting the external provider instead.
	config := auditProjectConfigWithExternalProvider("vendor-lint")
	request := auditRunnerRequestForProvider(t, "govard", config)
	selection, err := cmd.ResolveAuditLintSelectionForTest(request)
	if err != nil {
		t.Fatalf("ResolveAuditLintSelectionForTest returned error: %v", err)
	}
	if selection.External || selection.ExternalImage != "" || len(selection.ExternalCommand) != 0 {
		t.Fatalf("selection = %#v, want no external provider carried by a native selection", selection)
	}

	// A negative identity survives the "unset" guard and is rejected by the
	// native backend, so the failure branch is forced deterministically for any
	// user running the suite. Zero would not: it reads as "unset" and falls back
	// to os.Getuid(), which succeeds for an ordinary user and would leave this
	// guarantee unexercised.
	unusableConfig := auditProjectConfigWithExternalProvider("vendor-lint")
	unusableConfig.Stack.UserID = -1
	unusableConfig.Stack.GroupID = -1
	unusableRequest := auditRunnerRequestForProvider(t, "govard", unusableConfig)
	backend, err := cmd.NewAuditLintBackendForTest(unusableRequest)
	if err == nil {
		t.Fatalf("backend = %T, want a native construction failure for an unusable container identity", backend)
	}
	if backend != nil {
		t.Fatalf("backend = %T, want no backend when the native one cannot be constructed", backend)
	}
	if !strings.Contains(err.Error(), "host user and group IDs") {
		t.Fatalf("error = %v, want the native container-identity rejection", err)
	}
}

func TestAuditReadOnlyCommandsNeverSelectALintProvider(t *testing.T) {
	var captured []cmd.AuditRunnerRequest
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{
		RunnerFactory: func(request cmd.AuditRunnerRequest) (*audit.Runner, error) {
			captured = append(captured, request)
			return audit.NewRunner(audit.RunnerOptions{
				Store:       audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir())),
				LintBackend: &commandLintBackend{},
				Resources:   audit.Resources{CPU: 4, MemoryMB: 4096},
			}), nil
		},
	})
	t.Cleanup(restore)

	for name, testCase := range map[string]struct {
		arguments []string
		want      bool
	}{
		"run": {arguments: []string{"audit", "run", "--format", "json"}, want: true},
		// A rerun with explicit checks keeps the lint-backend contract; the
		// omitted-checks form peeks the session first and never constructs a
		// backend when the session does not exist.
		"rerun":   {arguments: []string{"audit", "rerun", "--session", "missing-session", "--checks", "lint"}, want: true},
		"status":  {arguments: []string{"audit", "status", "--session", "missing-session"}, want: false},
		"result":  {arguments: []string{"audit", "result", "--session", "missing-session", "--run", "missing-run"}, want: false},
		"cleanup": {arguments: []string{"audit", "cleanup", "--older-than", "1h"}, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			project := auditCommandProject(t, "magento2")
			captured = nil
			// Reading a stored session may legitimately fail (nothing is
			// stored here); only what the command asked the factory for matters.
			_, _ = executeAuditCommand(t, project, testCase.arguments)
			if len(captured) != 1 {
				t.Fatalf("runner factory calls = %d, want 1", len(captured))
			}
			if captured[0].LintBackendRequired != testCase.want {
				t.Fatalf("lint backend required = %t, want %t", captured[0].LintBackendRequired, testCase.want)
			}
			if !testCase.want && captured[0].LintProvider != "" {
				t.Fatalf("read-only command resolved lint provider %q, want none", captured[0].LintProvider)
			}
			if testCase.want && captured[0].LintProvider == "" {
				t.Fatal("a lint-executing command resolved no lint provider")
			}
		})
	}
}

func TestAuditReadOnlyCommandsRunWhenTheNativeBackendCannotBeConstructed(t *testing.T) {
	// The real runner factory: a project whose container identity the native
	// lint backend rejects must still be able to read persisted sessions, since
	// those commands touch no lint backend at all.
	project := auditCommandProjectWithLintConfig(t, "")
	path := filepath.Join(project, ".govard.yml")
	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(existing, []byte("  user_id: -1\n  group_id: -1\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{
		RuntimePHPProbe: func(context.Context, types.AuditTarget, engine.Config) (string, bool, error) {
			return "", false, nil
		},
	})
	t.Cleanup(restore)

	_, runErr := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if runErr == nil || !strings.Contains(runErr.Error(), "host user and group IDs") {
		t.Fatalf("audit run error = %v, want the native container-identity rejection", runErr)
	}
	_, resultErr := executeAuditCommand(t, project, []string{"audit", "result", "--session", "missing-session", "--run", "missing-run"})
	if resultErr == nil {
		t.Fatal("audit result unexpectedly succeeded for a missing session")
	}
	if strings.Contains(resultErr.Error(), "host user and group IDs") {
		t.Fatalf("audit result failed on lint backend construction it never needs: %v", resultErr)
	}
	_, cleanupErr := executeAuditCommand(t, project, []string{"audit", "cleanup", "--older-than", "1h", "--format", "json"})
	if cleanupErr != nil {
		t.Fatalf("audit cleanup error = %v, want success without any lint backend", cleanupErr)
	}
}

func TestAuditProviderSelectionForwardsSSHAgentOnlyWhenOptedIn(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "/run/user/1000/keyring/ssh")
	request := auditRunnerRequestForProvider(t, "govard", auditProjectConfigWithExternalProvider("vendor-lint"))
	request.AllowSSHAgent = true

	selection, err := cmd.ResolveAuditLintSelectionForTest(request)
	if err != nil {
		t.Fatalf("ResolveAuditLintSelectionForTest returned error: %v", err)
	}
	if !selection.AllowSSHAgent || selection.SSHAgent != "/run/user/1000/keyring/ssh" {
		t.Fatalf("selection = %#v, want opted-in agent forwarding", selection)
	}
}

func TestAuditProviderSelectionResolvesNoAgentWhenTheHostHasNone(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	request := auditRunnerRequestForProvider(t, "govard", auditProjectConfigWithExternalProvider("vendor-lint"))
	request.AllowSSHAgent = true

	selection, err := cmd.ResolveAuditLintSelectionForTest(request)
	if err != nil {
		t.Fatalf("ResolveAuditLintSelectionForTest returned error: %v", err)
	}
	if selection.SSHAgent != "" {
		t.Fatalf("ssh agent = %q, want none resolved", selection.SSHAgent)
	}
}

func TestAuditRunAppliesLintProviderPrecedence(t *testing.T) {
	for name, testCase := range map[string]struct {
		arguments []string
		want      string
	}{
		"configured provider without an explicit flag": {arguments: []string{"audit", "run", "--format", "json"}, want: "vendor-lint"},
		"explicit flag overrides the configuration":    {arguments: []string{"audit", "run", "--format", "json", "--lint-provider", "govard"}, want: "govard"},
	} {
		t.Run(name, func(t *testing.T) {
			project := auditCommandProjectWithLintConfig(t, auditExternalProviderYAML("vendor-lint", "vendor-lint"))
			var captured cmd.AuditRunnerRequest
			captureAuditRunnerRequest(t, &commandLintBackend{}, &captured)
			if _, err := executeAuditCommand(t, project, testCase.arguments); err != nil {
				t.Fatal(err)
			}
			if captured.LintProvider != testCase.want {
				t.Fatalf("effective lint provider = %q, want %q", captured.LintProvider, testCase.want)
			}
		})
	}
}

func TestAuditRunDefaultsToTheGovardProviderWithoutAuditConfiguration(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	var captured cmd.AuditRunnerRequest
	captureAuditRunnerRequest(t, &commandLintBackend{}, &captured)
	if _, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	if captured.LintProvider != "govard" {
		t.Fatalf("effective lint provider = %q, want %q", captured.LintProvider, "govard")
	}
	if captured.AllowSSHAgent {
		t.Fatal("ssh agent forwarding was requested without --allow-lint-ssh-agent")
	}
}

func TestAuditRunForwardsAllowLintSSHAgentFlag(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	var captured cmd.AuditRunnerRequest
	captureAuditRunnerRequest(t, &commandLintBackend{}, &captured)
	if _, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json", "--allow-lint-ssh-agent"}); err != nil {
		t.Fatal(err)
	}
	if !captured.AllowSSHAgent {
		t.Fatal("--allow-lint-ssh-agent did not reach the lint backend selection")
	}
}

func TestAuditRunBypassesLintResultCacheOnlyWhenRequested(t *testing.T) {
	for name, testCase := range map[string]struct {
		arguments []string
		want      bool
	}{
		"default":               {arguments: []string{"audit", "run", "--format", "json"}, want: false},
		"explicit cache bypass": {arguments: []string{"audit", "run", "--format", "json", "--no-lint-result-cache"}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			project := auditCommandProject(t, "magento2")
			backend := &commandLintBackend{}
			installAuditCommandDependencies(t, backend)
			if _, err := executeAuditCommand(t, project, testCase.arguments); err != nil {
				t.Fatal(err)
			}
			if len(backend.requests) != 1 {
				t.Fatalf("lint requests = %d, want 1", len(backend.requests))
			}
			if backend.requests[0].BypassResultCache != testCase.want {
				t.Fatalf("bypass result cache = %t, want %t", backend.requests[0].BypassResultCache, testCase.want)
			}
			// The reusable cache root must stay identical whether or not the
			// result cache is bypassed: the Composer download cache lives there
			// and must never be discarded by asking for fresh analysis.
			if want := audit.DefaultLintCacheRoot(engine.GovardHomeDir()); backend.requests[0].CacheRoot != want {
				t.Fatalf("lint cache root = %q, want %q", backend.requests[0].CacheRoot, want)
			}
		})
	}
}

func TestAuditCommandNoLongerAcceptsTheUnreleasedLintBackendFlag(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--lint-backend", "ci"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v, want an unknown-flag failure for the removed --lint-backend", err)
	}
	help, err := executeAuditCommand(t, project, []string{"audit", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(help), "lint-backend") {
		t.Fatalf("audit help still documents --lint-backend:\n%s", help)
	}
	if !strings.Contains(string(help), "lint-provider") {
		t.Fatalf("audit help does not document --lint-provider:\n%s", help)
	}
}

// auditTargetModeVocabulary mirrors every value the --mode flag accepts; the
// help text and the unknown-mode error must both document this exact set.
var auditTargetModeVocabulary = []string{"auto", "project", "module_in_project", "standalone"}

func TestAuditModeFlagHelpDocumentsEveryTargetMode(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	help, err := executeAuditCommand(t, project, []string{"audit", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	var modeUsage []string
	for _, line := range strings.Split(string(help), "\n") {
		if strings.Contains(line, "--mode") {
			modeUsage = append(modeUsage, line)
		}
	}
	if len(modeUsage) == 0 {
		t.Fatalf("audit help does not document a --mode flag:\n%s", help)
	}
	documented := strings.Join(modeUsage, "\n")
	for _, mode := range auditTargetModeVocabulary {
		if !strings.Contains(documented, mode) {
			t.Fatalf("audit --mode help does not document target mode %q; got:\n%s", mode, documented)
		}
	}
}

func TestAuditUnknownModeErrorNamesValidModes(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--mode", "module"})
	if err == nil {
		t.Fatal("an unknown audit target mode was accepted")
	}
	for _, mode := range auditTargetModeVocabulary {
		if !strings.Contains(err.Error(), mode) {
			t.Fatalf("unknown-mode error does not name valid mode %q: %v", mode, err)
		}
	}
}

func TestAuditUnknownModeIsRejectedWithTheModeVocabulary(t *testing.T) {
	// Outside any framework project: the mode vocabulary check must fire
	// before target resolution can produce its less actionable
	// "no framework can resolve audit target" failure.
	project := t.TempDir()
	installAuditCommandDependencies(t, &commandLintBackend{})
	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--mode", "module"})
	if err == nil {
		t.Fatal("an unknown audit target mode was accepted")
	}
	if !strings.Contains(err.Error(), "unknown audit target mode") {
		t.Fatalf("error does not name the unknown audit target mode problem: %v", err)
	}
	for _, mode := range auditTargetModeVocabulary {
		if !strings.Contains(err.Error(), mode) {
			t.Fatalf("unknown-mode error does not name valid mode %q: %v", mode, err)
		}
	}
}

func auditRunnerRequestForProvider(t *testing.T, provider string, config *engine.Config) cmd.AuditRunnerRequest {
	t.Helper()
	root := t.TempDir()
	return cmd.AuditRunnerRequest{
		ProjectRoot:  root,
		Target:       types.AuditTarget{Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject},
		Config:       config,
		LintProvider: provider,
	}
}

func auditProjectConfigWithExternalProvider(provider string) *engine.Config {
	config := &engine.Config{Framework: "magento2"}
	config.Stack.UserID = 4242
	config.Stack.GroupID = 4243
	config.Audit.Lint.Provider = provider
	config.Audit.Lint.ExternalProviders = map[string]engine.ExternalLintProviderConfig{
		provider: {Type: "docker", Image: "registry.example.test/" + provider + ":1.0", Command: []string{"lint"}},
	}
	return config
}

func auditExternalProviderYAML(selected, provider string) string {
	return "audit:\n  lint:\n    provider: " + selected + "\n    external_providers:\n      " + provider + ":\n        type: docker\n        image: registry.example.test/" + provider + ":1.0\n        command:\n          - lint\n"
}

func auditCommandProjectWithLintConfig(t *testing.T, lintConfig string) string {
	t.Helper()
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.3")
	path := filepath.Join(project, ".govard.yml")
	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(existing, []byte(lintConfig)...), 0o600); err != nil {
		t.Fatal(err)
	}
	return project
}

// installAuditCommandDependencies substitutes only the lint backend, so command
// tests still exercise the real runner construction (persisted store, reusable
// lint cache root) and the real provider selection.
func installAuditCommandDependencies(t *testing.T, backend audit.LintBackend) {
	t.Helper()
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{LintBackend: backend})
	t.Cleanup(restore)
}

func installAuditRunnerFactory(t *testing.T, factory func(cmd.AuditRunnerRequest) (*audit.Runner, error)) {
	t.Helper()
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{RunnerFactory: factory})
	t.Cleanup(restore)
}

// captureAuditRunnerRequest records the provider context the command resolved
// for the runner factory without changing any other real wiring.
func captureAuditRunnerRequest(t *testing.T, backend audit.LintBackend, captured *cmd.AuditRunnerRequest) {
	t.Helper()
	installAuditRunnerFactory(t, func(request cmd.AuditRunnerRequest) (*audit.Runner, error) {
		*captured = request
		return audit.NewRunner(audit.RunnerOptions{
			Store:       audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir())),
			LintBackend: backend,
			Resources:   audit.Resources{CPU: 4, MemoryMB: 4096},
		}), nil
	})
}

func auditCommandProject(t *testing.T, framework string) string {
	return auditCommandProjectWithPHPVersion(t, framework, "")
}

func auditCommandProjectWithPHPVersion(t *testing.T, framework, phpVersion string) string {
	t.Helper()
	project := t.TempDir()
	config := "project_name: audit-shop\ndomain: audit-shop.test\nframework: " + framework + "\n"
	if phpVersion != "" {
		config += "stack:\n  php_version: " + phpVersion + "\n"
	}
	if err := os.WriteFile(filepath.Join(project, ".govard.yml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if framework == "magento2" || framework == "mageos" {
		packageName := "magento/product-community-edition"
		if framework == "mageos" {
			packageName = "mage-os/product-community-edition"
		}
		composer := "{\"require\":{\"" + packageName + "\":\"*\"}}"
		if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte(composer), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(project, "bin"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "bin", "magento"), []byte("#!/usr/bin/env php\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return project
}

func executeAuditCommand(t *testing.T, project string, args []string) ([]byte, error) {
	t.Helper()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(project, "govard-home"))
	cmd.ResetAuditCommandForTest()
	root := cmd.RootCommandForTest()
	auditCommand, _, err := root.Find([]string{"audit"})
	if err != nil {
		t.Fatal(err)
	}
	auditCommand.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "php" || flag.Name == "checks" {
			flag.Changed = false
			return
		}
		value := flag.DefValue
		if err := flag.Value.Set(value); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	})
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err = root.Execute()
	return output.Bytes(), err
}
