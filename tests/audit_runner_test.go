package tests

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/frameworks/types"
)

type fakeLintBackend struct {
	requests []audit.LintRequest
	report   audit.LintReport
	err      error
}

func (backend *fakeLintBackend) Name() string { return "fake" }

func (backend *fakeLintBackend) Run(_ context.Context, request audit.LintRequest) (audit.LintReport, error) {
	backend.requests = append(backend.requests, request)
	return backend.report, backend.err
}

func TestAuditRunnerCreatesAndRerunsExplicitSession(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:       store,
		LintBackend: backend,
		Resources:   audit.Resources{CPU: 2, MemoryMB: 2048},
	})

	request := auditRunRequest()
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Rerun(context.Background(), first.SessionID, request.ProjectID, []string{"lint"})
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != second.SessionID || first.RunID == second.RunID {
		t.Fatalf("first=%s/%s second=%s/%s", first.SessionID, first.RunID, second.SessionID, second.RunID)
	}
}

func TestAuditRunnerRejectsImplicitLatestRerun(t *testing.T) {
	runner := audit.NewRunner(audit.RunnerOptions{Store: newDeterministicAuditStore(t), LintBackend: &fakeLintBackend{}})
	_, err := runner.Rerun(context.Background(), "", "project-aabbccdd", []string{"lint"})
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("empty session rerun error = %v", err)
	}
}

func TestAuditRunnerRerunUsesFrozenManifestAndRejectsProjectMismatch(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	request.Scope = audit.ScopeDiff
	request.BaseRef = "origin/master"
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runner.Rerun(context.Background(), first.SessionID, "project-other", []string{"lint"}); err == nil {
		t.Fatal("rerun accepted a different project")
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, request.ProjectID, []string{"lint"}); err != nil {
		t.Fatal(err)
	}
	last := backend.requests[len(backend.requests)-1]
	if last.ProjectRoot != request.ProjectRoot || last.Scope != audit.ScopeDiff || last.BaseRef != "origin/master" {
		t.Fatalf("rerun request = %#v, want frozen manifest values", last)
	}
}

func TestAuditRerunFindsLatestLintSettingsAfterProfilerOnlyRun(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	runtime := &fakeProfilerRuntime{}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	request.Checks = []string{"lint", "profiler"}
	request.ProfilerURL = "https://shop.test/"
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"profiler"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"lint"}); err != nil {
		t.Fatalf("lint rerun after profiler-only run = %v", err)
	}
	if len(backend.requests) != 2 {
		t.Fatalf("lint backend calls = %d, want two", len(backend.requests))
	}
}

func TestAuditProfilerOnlyRerunPreservesProjectTarget(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	runtime := &fakeProfilerRuntime{}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	request.Checks = []string{"lint", "profiler"}
	request.ProfilerURL = "https://shop.test/"
	request.Target = types.AuditTarget{Framework: "magento2", ProjectRoot: "/work/shop", TargetPath: "/work/shop", Mode: types.AuditTargetProject}
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"profiler"}); err != nil {
		t.Fatal(err)
	}
	if got := runtime.activateRequests[len(runtime.activateRequests)-1].Target; !reflect.DeepEqual(got, request.Target) {
		t.Fatalf("profiler rerun target = %#v, want %#v", got, request.Target)
	}
}

func TestAuditRunnerPersistsLintFailureAndInfrastructureError(t *testing.T) {
	t.Run("lint failure", func(t *testing.T) {
		backend := &fakeLintBackend{report: failedLintReport("project-aabbccdd")}
		store := newDeterministicAuditStore(t)
		runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
		result, err := runner.Run(context.Background(), auditRunRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != audit.StatusFailed || len(result.Errors) != 0 {
			t.Fatalf("result = %#v, want failed result without infrastructure error", result)
		}
		persisted, err := store.ReadResult(result.ProjectID, result.SessionID, result.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Status != audit.StatusFailed {
			t.Fatalf("persisted status = %q", persisted.Status)
		}
	})

	t.Run("infrastructure error", func(t *testing.T) {
		backend := &fakeLintBackend{err: errors.New("docker unavailable")}
		store := newDeterministicAuditStore(t)
		runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
		result, err := runner.Run(context.Background(), auditRunRequest())
		if err == nil || !strings.Contains(err.Error(), "docker unavailable") {
			t.Fatalf("run error = %v", err)
		}
		if result.Status != audit.StatusFailed || len(result.Errors) != 1 {
			t.Fatalf("result = %#v, want persisted infrastructure failure", result)
		}
	})
}

func TestAuditRunnerPersistsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: &fakeLintBackend{}, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	result, err := runner.Run(ctx, auditRunRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != audit.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
	persisted, err := store.ReadResult(result.ProjectID, result.SessionID, result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != audit.StatusCancelled {
		t.Fatalf("persisted status = %q, want cancelled", persisted.Status)
	}
}

func TestAuditRunnerRejectsUnknownChecksAndNormalizesDuplicates(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: newDeterministicAuditStore(t), LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	invalid := auditRunRequest()
	invalid.Checks = []string{"lint", "browser"}
	if _, err := runner.Run(context.Background(), invalid); err == nil {
		t.Fatal("unknown check was accepted")
	}
	request := auditRunRequest()
	request.Checks = []string{"lint", "lint"}
	if _, err := runner.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(backend.requests) != 1 {
		t.Fatalf("backend calls = %d, want one normalized lint job", len(backend.requests))
	}
}

func TestAuditRunnerRejectsAggregateStatusThatConflictsWithPHPResults(t *testing.T) {
	report := passingLintReport("project-aabbccdd")
	report.PHPResults[0].Outcome = "failed"
	report.PHPResults[0].Phases[0].Status = "failed"
	runner := audit.NewRunner(audit.RunnerOptions{Store: newDeterministicAuditStore(t), LintBackend: &fakeLintBackend{report: report}, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	if _, err := runner.Run(context.Background(), auditRunRequest()); err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("mismatched aggregate status error = %v", err)
	}
}

func TestAuditRunnerRerunPreservesHistoricalLintPHPMatrix(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.ReadResult(first.ProjectID, first.SessionID, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	persisted.Jobs[0].Evidence["lint_settings"] = map[string]any{
		"profile": map[string]any{
			"PHPVersions":  []string{"7.4", "8.1"},
			"Linters":      []string{"phpcs", "phpstan"},
			"PHPStanLevel": 5,
		},
		"jobs": request.LintJobs,
	}
	if err := store.WriteResult(persisted); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"lint"}); err != nil {
		t.Fatal(err)
	}
	last := backend.requests[len(backend.requests)-1]
	if got, want := last.SelectedPHPVersions, []string{"7.4", "8.1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("historical selected PHP versions = %#v, want %#v", got, want)
	}
	if !last.MatrixComplete {
		t.Fatal("historical full matrix was not preserved as complete")
	}
}

func TestAuditRerunPreservesLintSettings(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	store := newDeterministicAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	request.ProjectRoot = "/work/module"
	request.Target = types.AuditTarget{
		Framework:  "magento2",
		TargetPath: "/work/module",
		Mode:       types.AuditTargetStandalone,
	}
	request.LintProfile.StandalonePHPVersions = []string{"8.1", "8.2", "8.3", "8.4", "8.5"}
	request.SelectedPHPVersions = []string{"8.1", "8.5"}
	request.MatrixComplete = false
	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"lint"}); err != nil {
		t.Fatal(err)
	}
	last := backend.requests[len(backend.requests)-1]
	if !reflect.DeepEqual(last.Target, request.Target) {
		t.Fatalf("rerun target = %#v, want %#v", last.Target, request.Target)
	}
	if got, want := last.SelectedPHPVersions, request.SelectedPHPVersions; !reflect.DeepEqual(got, want) {
		t.Fatalf("rerun selected PHP versions = %#v, want %#v", got, want)
	}
	if last.MatrixComplete != request.MatrixComplete {
		t.Fatalf("rerun matrix complete = %t, want %t", last.MatrixComplete, request.MatrixComplete)
	}
}

func TestAuditRunnerDerivesTargetIDPerResolvedTarget(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	store := newSequentialAuditStore(t)
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	project := auditRunRequest()
	project.Target = types.AuditTarget{Framework: "magento2", ProjectRoot: "/work/shop", TargetPath: "/work/shop", Mode: types.AuditTargetProject}
	module := auditRunRequest()
	module.Target = types.AuditTarget{Framework: "magento2", ProjectRoot: "/work/shop", TargetPath: "/work/shop/app/code/Vendor/Module", Mode: types.AuditTargetModule}
	standalone := auditRunRequest()
	standalone.Target = types.AuditTarget{Framework: "magento2", TargetPath: "/work/module", Mode: types.AuditTargetStandalone}
	standalone.LintProfile.StandalonePHPVersions = []string{"8.4"}

	first, err := runner.Run(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []audit.RunRequest{module, standalone} {
		if _, err := runner.Run(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"lint"}); err != nil {
		t.Fatal(err)
	}
	if len(backend.requests) != 4 {
		t.Fatalf("backend calls = %d, want four lint requests", len(backend.requests))
	}
	identifier := regexp.MustCompile(`^target-[0-9a-f]{32}$`)
	seen := make(map[string]int, len(backend.requests))
	for index, request := range backend.requests {
		if !identifier.MatchString(request.TargetID) {
			t.Fatalf("target ID %d = %q, want an opaque derived identifier", index, request.TargetID)
		}
		if strings.Contains(request.TargetID, "project-") {
			t.Fatalf("target ID %d reuses the project identifier: %q", index, request.TargetID)
		}
		seen[request.TargetID]++
	}
	if len(seen) != 3 {
		t.Fatalf("distinct target IDs = %d, want one per resolved target: %#v", len(seen), seen)
	}
	if seen[backend.requests[0].TargetID] != 2 || backend.requests[0].TargetID != backend.requests[3].TargetID {
		t.Fatalf("rerunning the same target changed its target ID: %#v", seen)
	}
}

func TestAuditRunnerSurfacesLintReportOutcomes(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		report    audit.LintReport
		status    audit.Status
		errors    int
		errorCode string
		message   string
	}{
		{name: "infrastructure error", report: infrastructureLintReport("project-aabbccdd"), status: audit.StatusFailed, errors: 1, errorCode: "infrastructure", message: "missing analyzer toolchain"},
		{name: "unsupported policy", report: unsupportedLintReport("project-aabbccdd"), status: audit.StatusPassed},
		{name: "cancelled by signal", report: cancelledLintReport("project-aabbccdd"), status: audit.StatusCancelled},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := newDeterministicAuditStore(t)
			runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: &fakeLintBackend{report: testCase.report}, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
			result, err := runner.Run(context.Background(), auditRunRequest())
			if testCase.errors == 0 && err != nil {
				t.Fatalf("run error = %v, want a policy outcome without an audit error", err)
			}
			if testCase.errors > 0 && err == nil {
				t.Fatal("an infrastructure report was reported as success")
			}
			if result.Status != testCase.status {
				t.Fatalf("status = %q, want %q", result.Status, testCase.status)
			}
			if len(result.Errors) != testCase.errors {
				t.Fatalf("errors = %#v, want %d", result.Errors, testCase.errors)
			}
			if testCase.errors > 0 {
				if result.Errors[0].Code != testCase.errorCode || !strings.Contains(result.Errors[0].Message, testCase.message) {
					t.Fatalf("audit error = %#v, want code %q mentioning %q", result.Errors[0], testCase.errorCode, testCase.message)
				}
			}
			persisted, readErr := store.ReadResult(result.ProjectID, result.SessionID, result.RunID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if persisted.Status != testCase.status {
				t.Fatalf("persisted status = %q, want %q", persisted.Status, testCase.status)
			}
		})
	}
}

func TestAuditRunnerKeepsCancelledLintBackendOutOfAuditErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newDeterministicAuditStore(t)
	backend := &fakeLintBackend{report: audit.LintReport{Status: "cancelled"}, err: errors.New("govard lint cancelled: context canceled")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	result, err := runner.Run(ctx, auditRunRequest())
	if err != nil {
		t.Fatalf("run error = %v, want a cancellation represented in the result only", err)
	}
	if result.Status != audit.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v, want no structured infrastructure error for a cancellation", result.Errors)
	}
}

func TestAuditRunnerForwardsResultCacheBypass(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: newDeterministicAuditStore(t), LintBackend: backend, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	request := auditRunRequest()
	request.BypassResultCache = true
	if _, err := runner.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !backend.requests[0].BypassResultCache {
		t.Fatal("the result-cache bypass did not reach the lint backend")
	}
}

func TestAuditRunnerUsesConfiguredLintCacheRoot(t *testing.T) {
	backend := &fakeLintBackend{report: passingLintReport("project-aabbccdd")}
	cacheRoot := audit.DefaultLintCacheRoot(t.TempDir())
	runner := audit.NewRunner(audit.RunnerOptions{Store: newDeterministicAuditStore(t), LintBackend: backend, LintCacheRoot: cacheRoot, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	if _, err := runner.Run(context.Background(), auditRunRequest()); err != nil {
		t.Fatal(err)
	}
	if backend.requests[0].CacheRoot != cacheRoot {
		t.Fatalf("lint cache root = %q, want %q", backend.requests[0].CacheRoot, cacheRoot)
	}
}

func auditRunRequest() audit.RunRequest {
	return audit.RunRequest{
		ProjectRoot:         "/work/shop",
		ProjectID:           "project-aabbccdd",
		Scope:               audit.ScopeProject,
		Checks:              []string{"lint"},
		LintJobs:            2,
		Environment:         audit.EnvironmentFingerprint{Framework: "sample", GovardVersion: "test"},
		Source:              audit.SourceFingerprint{Digest: "sha256:source"},
		LintProfile:         types.AuditLintProfile{ProjectPHPVersions: []string{"8.4"}, Linters: []string{"phpcs", "phpstan"}, PHPStanLevel: 5},
		SelectedPHPVersions: []string{"8.4"},
		MatrixComplete:      true,
	}
}

func passingLintReport(projectID string) audit.LintReport {
	return audit.LintReport{SchemaVersion: audit.LintReportSchemaVersion, Provider: "ci", ProjectID: projectID, ImageDigest: "sha256:image", ToolchainDigest: "sha256:toolchain", Status: "passed", PHPResults: []audit.LintPHPResult{{PHPVersion: "8.4", Outcome: "passed", Phases: []audit.LintPhase{{Name: "phpcs", Status: "passed"}}}}}
}

func failedLintReport(projectID string) audit.LintReport {
	report := passingLintReport(projectID)
	report.Status = "failed"
	report.PHPResults[0].Outcome = "failed"
	report.PHPResults[0].Phases[0].Status = "failed"
	return report
}

func infrastructureLintReport(projectID string) audit.LintReport {
	report := passingLintReport(projectID)
	report.Status = "infra_error"
	report.PHPResults[0].Outcome = "infra_error"
	report.PHPResults[0].Phases[0].Status = "error"
	report.PHPResults[0].Phases[0].CacheReason = "missing analyzer toolchain"
	report.PHPResults[0].Cache = audit.CacheOutcome{State: "cold", Reason: "missing analyzer toolchain"}
	return report
}

func unsupportedLintReport(projectID string) audit.LintReport {
	report := passingLintReport(projectID)
	report.Status = "unsupported"
	report.PHPResults[0].Outcome = "unsupported"
	report.PHPResults[0].Phases[0].Status = "unsupported"
	return report
}

func cancelledLintReport(projectID string) audit.LintReport {
	report := passingLintReport(projectID)
	report.Status = "cancelled"
	report.PHPResults[0].Outcome = "cancelled"
	report.PHPResults[0].Phases[0].Status = "cancelled"
	return report
}

// newSequentialAuditStore keeps the deterministic clock but issues a distinct
// session ID per call, so one test can create several sessions in one store.
func newSequentialAuditStore(t *testing.T) *audit.Store {
	t.Helper()
	var counter byte
	return audit.NewStore(t.TempDir(), audit.WithClock(func() time.Time { return time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC) }), audit.WithRandomBytes(func(buffer []byte) error {
		counter++
		for index := range buffer {
			buffer[index] = counter
		}
		return nil
	}))
}

func newDeterministicAuditStore(t *testing.T) *audit.Store {
	t.Helper()
	return audit.NewStore(t.TempDir(), audit.WithClock(func() time.Time { return time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC) }), audit.WithRandomBytes(func(buffer []byte) error {
		copy(buffer, []byte{1, 2, 3, 4})
		return nil
	}))
}
