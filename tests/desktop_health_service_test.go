package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/desktop"
	"govard/internal/engine"
)

// setupHealthServiceTest isolates the health service: fresh preview tokens,
// empty op registry, and a private Govard home so the audit banner index
// never touches the developer machine.
func setupHealthServiceTest(t *testing.T) {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetDoctorServiceForTest()
	desktop.ResetAuditServiceForTest()
	desktop.ResetOperationRegistryForTest()
	t.Cleanup(desktop.ResetDoctorServiceForTest)
	t.Cleanup(desktop.ResetAuditServiceForTest)
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

func newHealthServiceForTest() *desktop.HealthService {
	return desktop.NewApp(desktop.WithPlatform(&desktop.FakePlatform{})).Health
}

// The health service delegates the doctor run: the banner gets the same
// classified report the package function returns, not a stub error.
func TestHealthService_RunDoctorDelegates(t *testing.T) {
	setupHealthServiceTest(t)
	stubDoctorDiagnostics(t)

	svc := newHealthServiceForTest()
	report, err := svc.RunDoctor()
	if err != nil {
		t.Fatalf("RunDoctor() error = %v, want a classified report", err)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("RunDoctor() returned %d checks, want 4", len(report.Checks))
	}
	if got := doctorFixKinds(t, report); !equalFixKinds(got, map[string]desktop.DoctorFixKind{
		"host.govard.home":     desktop.DoctorFixKindSafe,
		"project.config.drift": desktop.DoctorFixKindRisky,
		"host.port.80":         desktop.DoctorFixKindNone,
		"docker.daemon":        desktop.DoctorFixKindNone,
	}) {
		t.Fatalf("fix kinds = %v, want the allowlist classification", got)
	}
}

// A safe fix applies 1-click through the service: no preview token, no
// confirm flag, and the stub executor observes one real (non-dry-run) call.
func TestHealthService_SafeFixAppliesThroughService(t *testing.T) {
	setupHealthServiceTest(t)
	var calls int
	var sawDryRun bool
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			calls++
			sawDryRun = sawDryRun || dryRun
			return []string{"did fix"}, "fixed", nil
		}))

	svc := newHealthServiceForTest()
	outcome, err := svc.ApplyDoctorFix("", "host.govard.home", "", false)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(safe) error = %v, want 1-click apply", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied {
		t.Fatalf("safe fix status = %q, want applied", outcome.Status)
	}
	if calls != 1 || sawDryRun {
		t.Fatalf("executor calls = %d dryRunSeen = %v, want exactly 1 real call", calls, sawDryRun)
	}
}

// A risky fix through the service always opens a preview first: apply
// without the preview token is refused, and the token plus an explicit
// confirm applies. Risky fixes are project-scoped, so the flow runs against
// a temp project root holding a legacy config (scoped runs use the real
// dispatcher; the stub seam is for hermetic unscoped tests only).
func TestHealthService_RiskyFixPreviewThenApply(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}

	svc := newHealthServiceForTest()
	if _, err := svc.ApplyDoctorFix(root, "project.config.audit", "", false); err == nil {
		t.Fatalf("ApplyDoctorFix(risky, no token no confirm) = nil error, want a preview-token refusal")
	}
	preview, err := svc.PreviewDoctorFix(root, "project.config.audit")
	if err != nil {
		t.Fatalf("PreviewDoctorFix(risky) error = %v, want a dry-run plan", err)
	}
	if preview.PreviewID == "" || !preview.RequiresConfirm {
		t.Fatalf("preview = %+v, want a token plus a confirm requirement", preview)
	}
	outcome, err := svc.ApplyDoctorFix(root, "project.config.audit", preview.PreviewID, true)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(risky, token+confirm) error = %v, want apply", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied {
		t.Fatalf("risky fix status = %q, want applied", outcome.Status)
	}
}

// A check with no fix exposes no fix path through the service either.
func TestHealthService_NoneCheckHasNoFixPath(t *testing.T) {
	setupHealthServiceTest(t)

	svc := newHealthServiceForTest()
	if _, err := svc.PreviewDoctorFix("", "host.port.80"); err == nil {
		t.Fatalf("PreviewDoctorFix(none) = nil error, want a no-fix refusal")
	} else if !strings.Contains(err.Error(), "no fix") {
		t.Fatalf("PreviewDoctorFix(none) error = %v, want a no-fix refusal", err)
	}
	if _, err := svc.ApplyDoctorFix("", "host.port.80", "", true); err == nil {
		t.Fatalf("ApplyDoctorFix(none) = nil error, want a no-fix refusal")
	} else if !strings.Contains(err.Error(), "no fix") {
		t.Fatalf("ApplyDoctorFix(none) error = %v, want a no-fix refusal", err)
	}
}

// Project-root scoping: the config-audit fix reads and plans against the
// passed root, never the process working directory. The test process runs
// in tests/ (which has no .govard.yml), so a CWD-scoped executor fails
// while a root-scoped one plans the migration in the temp root.
func TestHealthService_ProjectRootScopesConfigAudit(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}

	preview, err := desktop.PreviewDoctorFixForProject("project.config.audit", root)
	if err != nil {
		t.Fatalf("PreviewDoctorFixForProject() error = %v, want a migration plan for the temp root", err)
	}
	if len(preview.Actions) == 0 {
		t.Fatalf("preview actions empty, want the legacy-key migration plan")
	}
}

// The full preview-to-apply flow honors the root on write too: the temp
// root's config migrates while anything outside it is untouched.
func TestHealthService_ProjectRootScopesConfigAuditApply(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}

	svc := newHealthServiceForTest()
	preview, err := svc.PreviewDoctorFix(root, "project.config.audit")
	if err != nil {
		t.Fatalf("PreviewDoctorFix(root) error = %v, want a plan", err)
	}
	outcome, err := svc.ApplyDoctorFix(root, "project.config.audit", preview.PreviewID, true)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(root, token+confirm) error = %v, want apply", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied {
		t.Fatalf("apply status = %q, want applied", outcome.Status)
	}
	updated, err := os.ReadFile(filepath.Join(root, ".govard.yml"))
	if err != nil {
		t.Fatalf("read migrated .govard.yml: %v", err)
	}
	if strings.Contains(string(updated), "db: mariadb") && !strings.Contains(string(updated), "services:") {
		t.Fatalf("temp .govard.yml still carries the legacy key, want it migrated")
	}
}

// The integrity audit delegates through the service: docker-free run,
// integrity row enabled, docker-backed rows present but disabled with a
// reason (never hidden).
func TestHealthService_RunIntegrityAuditDelegates(t *testing.T) {
	setupHealthServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	svc := newHealthServiceForTest()
	view, err := svc.RunIntegrityAudit("")
	if err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v, want a result view", err)
	}
	if view.SessionID != "sess-1" {
		t.Fatalf("audit session = %q, want sess-1", view.SessionID)
	}
	integrity := auditCheckByID(t, view, audit.IntegrityCheck)
	if !integrity.Enabled {
		t.Fatalf("integrity check %+v, want enabled", integrity)
	}
	for _, id := range []string{"lint", "profiler"} {
		check := auditCheckByID(t, view, id)
		if check.Enabled || strings.TrimSpace(check.DisabledReason) == "" {
			t.Fatalf("docker-backed check %+v, want disabled with reason", check)
		}
	}
}

// The banner index is readable through the service: after a completed run
// the last result resolves, and a project with no run reports "no completed
// audit", never the stub sentinel.
func TestHealthService_LastAuditResultReadsBanner(t *testing.T) {
	setupHealthServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	svc := newHealthServiceForTest()
	if _, err := svc.RunIntegrityAudit(""); err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	view, err := svc.LastAuditResult("")
	if err != nil {
		t.Fatalf("LastAuditResult() error = %v, want the recorded run", err)
	}
	if view.SessionID != "sess-1" {
		t.Fatalf("banner session = %q, want sess-1", view.SessionID)
	}
	if _, err := svc.LastAuditResult(t.TempDir()); err == nil {
		t.Fatalf("LastAuditResult(fresh root) = nil error, want a no-completed-audit refusal")
	} else if strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("LastAuditResult(fresh root) error = %v, want a real no-result refusal", err)
	}
}

// The stored-run diff delegates through the service: previous-vs-current
// summary over two stubbed runs.
func TestHealthService_GetAuditDiffDelegates(t *testing.T) {
	setupHealthServiceTest(t)
	findingA := audit.LintFinding{Tool: "composer", Rule: "hash", Path: "composer.json", Message: "stale hash"}
	findingB := audit.LintFinding{Tool: "composer", Rule: "version", Path: "composer.json", Message: "old lock"}
	findingC := audit.LintFinding{Tool: "magento", Rule: "mode", Path: "app/etc/env.php", Message: "developer mode"}
	runWith := func(id string, findings ...audit.LintFinding) audit.RunResult {
		result := passedIntegrityResult()
		result.RunID = id
		result.Jobs = []audit.JobResult{{
			ID:     audit.IntegrityCheck,
			Status: audit.StatusFailed,
			Evidence: map[string]any{
				"findings": findings,
			},
		}}
		result.Status = audit.StatusFailed
		return result
	}
	t.Cleanup(desktop.SetAuditRunListForDesktopForTest(
		func(projectID, sessionID string) ([]string, error) {
			return []string{"1", "2"}, nil
		}))
	t.Cleanup(desktop.SetAuditResultReaderForDesktopForTest(
		func(projectID, sessionID, runID string) (audit.RunResult, error) {
			if runID == "1" {
				return runWith("1", findingA, findingB), nil
			}
			return runWith("2", findingB, findingC), nil
		}))

	svc := newHealthServiceForTest()
	diff, err := svc.GetAuditDiff("project-test", "sess-1", "", "")
	if err != nil {
		t.Fatalf("GetAuditDiff() error = %v, want a summary diff", err)
	}
	if len(diff.Added) != 1 || len(diff.Removed) != 1 || diff.UnchangedCount != 1 {
		t.Fatalf("diff added=%d removed=%d unchanged=%d, want 1/1/1", len(diff.Added), len(diff.Removed), diff.UnchangedCount)
	}
}

// Cancelling through the service stops the in-flight run: true for a live
// op with a discard error, false for an unknown one.
func TestHealthService_CancelDelegates(t *testing.T) {
	setupHealthServiceTest(t)
	started := make(chan struct{})
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			close(started)
			<-ctx.Done()
			return audit.RunResult{}, ctx.Err()
		}))

	svc := newHealthServiceForTest()
	done := make(chan error, 1)
	go func() {
		_, err := svc.RunIntegrityAudit("")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("audit runner never started; RunIntegrityAudit did not reach it")
	}
	var opID string
	for _, op := range desktop.ListOps() {
		if op.Kind == "audit" {
			opID = op.ID
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during run has no audit op: %+v", desktop.ListOps())
	}
	if !svc.CancelAudit(opID) {
		t.Fatalf("CancelAudit(running run) = false, want true")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("cancelled run error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "cancel") {
			t.Fatalf("cancelled run error = %v, want a cancellation error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled run did not return")
	}
	if svc.CancelAudit("audit:does-not-exist") {
		t.Fatalf("CancelAudit(unknown op) = true, want false")
	}
}

// Cancelling a live doctor run through the service releases it: the stub
// diagnostics blocks forever, so only a real registry cancel lets the run
// return with a cancellation error.
func TestHealthService_CancelDoctorRunDelegates(t *testing.T) {
	setupHealthServiceTest(t)
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		select {}
	}))

	svc := newHealthServiceForTest()
	done := make(chan error, 1)
	go func() {
		_, err := svc.RunDoctor()
		done <- err
	}()
	// The blocking stub never returns, so the op must still register before
	// the run blocks: poll the registry briefly instead of waiting on it.
	var opID string
	deadline := time.Now().Add(5 * time.Second)
	for opID == "" && time.Now().Before(deadline) {
		for _, op := range desktop.ListOps() {
			if op.Kind == "doctor" {
				opID = op.ID
			}
		}
		if opID == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during run has no doctor op: %+v", desktop.ListOps())
	}
	if !svc.CancelDoctorRun(opID) {
		t.Fatalf("CancelDoctorRun(live run) = false, want true")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("cancelled doctor run error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "cancel") {
			t.Fatalf("cancelled doctor run error = %v, want a cancellation error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled doctor run did not return")
	}
	if svc.CancelDoctorRun("doctor:does-not-exist") {
		t.Fatalf("CancelDoctorRun(unknown op) = true, want false")
	}
}
