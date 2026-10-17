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

// Review fix 1: a project-touching fix with no project selected must hard
// error guiding the user to select a project, never silently operate on the
// app's working directory. Host-global fixes may still run explicitly
// unscoped.
func TestHealthService_UnscopedProjectFixNeedsProject(t *testing.T) {
	setupHealthServiceTest(t)

	svc := newHealthServiceForTest()
	if _, err := svc.PreviewDoctorFix("", "project.config.audit"); err == nil {
		t.Fatalf("PreviewDoctorFix(unscoped project fix) = nil error, want a select-a-project refusal")
	} else if !strings.Contains(err.Error(), "select a project") {
		t.Fatalf("PreviewDoctorFix(unscoped project fix) error = %v, want select-a-project guidance", err)
	}
	if _, err := svc.ApplyDoctorFix("", "project.config.audit", "", true); err == nil {
		t.Fatalf("ApplyDoctorFix(unscoped project fix) = nil error, want a select-a-project refusal")
	} else if !strings.Contains(err.Error(), "select a project") {
		t.Fatalf("ApplyDoctorFix(unscoped project fix) error = %v, want select-a-project guidance", err)
	}

	// Host-global fixes are unaffected: the safe refusal below proves the
	// call reached classification, not the scope guard.
	if _, err := svc.PreviewDoctorFix("", "host.govard.home"); err == nil {
		t.Fatalf("PreviewDoctorFix(unscoped host-global safe) = nil error, want a direct-apply refusal")
	} else if !strings.Contains(err.Error(), "applies directly") {
		t.Fatalf("PreviewDoctorFix(unscoped host-global safe) error = %v, want a direct-apply refusal", err)
	}
}

// Review fix 1 (second half): a project argument that resolves to no
// directory must also refuse a project-touching fix, never fall back to the
// working directory.
func TestHealthService_UnresolvableProjectFixRefused(t *testing.T) {
	setupHealthServiceTest(t)

	svc := newHealthServiceForTest()
	if _, err := svc.PreviewDoctorFix("does-not-exist-project-xyz", "project.config.audit"); err == nil {
		t.Fatalf("PreviewDoctorFix(unresolvable project) = nil error, want a refusal, never a CWD fallback")
	}
	if _, err := svc.ApplyDoctorFix("does-not-exist-project-xyz", "project.config.audit", "", true); err == nil {
		t.Fatalf("ApplyDoctorFix(unresolvable project) = nil error, want a refusal, never a CWD fallback")
	}
}

// I2: a non-empty project that resolves to no directory must hard-error
// with select guidance instead of silently auditing the process working
// directory. This mirrors the fix-path hard error in resolveHealthFixRoot:
// the seeded registry entry matches by name but its relative path is no
// directory, so resolution yields no root ("",nil) and the run must refuse
// before the runner ever starts.
func TestHealthService_UnresolvableProjectAuditRefused(t *testing.T) {
	setupHealthServiceTest(t)
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		ProjectName: "ghost-project",
		Path:        "relative/ghost-path",
	}); err != nil {
		t.Fatalf("seed registry with relative-path entry: %v", err)
	}
	runnerStarted := false
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			runnerStarted = true
			return passedIntegrityResult(), nil
		}))

	svc := newHealthServiceForTest()
	if _, err := svc.RunIntegrityAudit("ghost-project"); err == nil {
		t.Fatalf("RunIntegrityAudit(unresolvable project) = nil error, want a select-guidance refusal, never a CWD audit")
	} else if !strings.Contains(strings.ToLower(err.Error()), "select a project") &&
		!strings.Contains(strings.ToLower(err.Error()), "did not resolve") {
		t.Fatalf("RunIntegrityAudit(unresolvable project) error = %v, want select/resolve guidance", err)
	}
	if runnerStarted {
		t.Fatalf("audit runner started for an unresolvable project, want refusal before any run")
	}
}

// Review fix 3: the panel recovers the in-flight audit op id on mount via a
// backend query, so Cancel is available even when the run started before the
// panel mounted. The query is project-scoped: another root reports no op.
func TestHealthService_RunningAuditOpRecoversInFlightOp(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	other := t.TempDir()
	// Service-level resolution only accepts a raw path that holds a project
	// config, so both scopes get one (content is irrelevant: the runner and
	// the resolution are stubbed).
	for _, dir := range []string{root, other} {
		if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
			t.Fatalf("write temp .govard.yml: %v", err)
		}
	}
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
		_, err := svc.RunIntegrityAudit(root)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("audit runner never started; RunIntegrityAudit did not reach it")
	}

	opID, err := svc.RunningAuditOp(root)
	if err != nil {
		t.Fatalf("RunningAuditOp(running scope) error = %v, want the in-flight op id", err)
	}
	if opID == "" {
		t.Fatalf("RunningAuditOp(running scope) = empty, want the in-flight audit op id")
	}
	if otherID, err := svc.RunningAuditOp(other); err != nil {
		t.Fatalf("RunningAuditOp(other scope) error = %v, want empty with nil error", err)
	} else if otherID != "" {
		t.Fatalf("RunningAuditOp(other scope) = %q, want empty (never another scope's op)", otherID)
	}

	if !svc.CancelAudit(opID) {
		t.Fatalf("CancelAudit(recovered op) = false, want true")
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
			t.Fatalf("cancelled run error = %v, want a cancellation error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled run did not return")
	}

	if opID, err := svc.RunningAuditOp(root); err != nil || opID != "" {
		t.Fatalf("RunningAuditOp(finished scope) = (%q, %v), want (empty, nil)", opID, err)
	}
}

// Review fix 1 at the scoped package API: an explicit empty root is the same
// unscoped call and must refuse project-touching fixes the same way.
func TestHealthFixForProject_EmptyRootNeedsProject(t *testing.T) {
	setupHealthServiceTest(t)

	if _, err := desktop.PreviewDoctorFixForProject("project.config.audit", ""); err == nil {
		t.Fatalf("PreviewDoctorFixForProject(empty root) = nil error, want a select-a-project refusal")
	} else if !strings.Contains(err.Error(), "select a project") {
		t.Fatalf("PreviewDoctorFixForProject(empty root) error = %v, want select-a-project guidance", err)
	}
	if _, err := desktop.ApplyDoctorFixForProject("project.config.audit", "", true, ""); err == nil {
		t.Fatalf("ApplyDoctorFixForProject(empty root) = nil error, want a select-a-project refusal")
	} else if !strings.Contains(err.Error(), "select a project") {
		t.Fatalf("ApplyDoctorFixForProject(empty root) error = %v, want select-a-project guidance", err)
	}

	// A scoped root still plans fine (control case: the guard only fires
	// when the scope is empty).
	root := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}
	if _, err := desktop.PreviewDoctorFixForProject("project.config.audit", root); err != nil {
		t.Fatalf("PreviewDoctorFixForProject(scoped root) error = %v, want a plan", err)
	}
}
