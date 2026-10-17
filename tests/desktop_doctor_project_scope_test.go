package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// Merged from the divergent remote worker and adapted to the local run
// mechanism. Remote threaded the project root as an engine argument
// (RunDoctorDiagnosticsInRoot); local scopes by entering the project
// directory for the run (reviewed, pinned by
// TestDoctorService_RunScopedToProjectRoot) with the engine-level InRoot
// helpers kept for direct callers. The intents below are the remote ones,
// asserted through the local seam: the scoped run diagnoses the selected
// project's root, never the process working directory.

// I1 (service half): HealthService.RunDoctor takes the selected project and
// runs the scoped diagnostics in its resolved root. The scoped run audits
// that root, so the report the UI buckets under the project actually
// describes the project, never the process working directory.
func TestHealthService_RunDoctorThreadsProjectRoot(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}
	var seen string
	called := false
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		seen, _ = os.Getwd()
		called = true
		return stubDoctorServiceReport()
	}))

	svc := newHealthServiceForTest()
	report, err := svc.RunDoctor(root)
	if err != nil {
		t.Fatalf("RunDoctor(root) error = %v, want a classified report", err)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("RunDoctor(root) returned %d checks, want 4", len(report.Checks))
	}
	if !called {
		t.Fatalf("scoped diagnostics never ran for the selected project")
	}
	want, _ := filepath.EvalSymlinks(root)
	got, _ := filepath.EvalSymlinks(seen)
	if got != want {
		t.Fatalf("scoped run diagnosed %q, want the selected project's root %q", seen, root)
	}
}

// I1 (misattribution guard): a non-empty project that resolves to no
// directory refuses with select guidance instead of auditing CWD and
// bucketing the host report under the project.
func TestHealthService_RunDoctorUnresolvableProjectRefused(t *testing.T) {
	setupHealthServiceTest(t)
	called := false
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		called = true
		return stubDoctorServiceReport()
	}))

	svc := newHealthServiceForTest()
	if _, err := svc.RunDoctor("does-not-exist-project-xyz"); err == nil {
		t.Fatalf("RunDoctor(unresolvable project) = nil error, want a refusal, never a CWD run")
	} else if !strings.Contains(strings.ToLower(err.Error()), "select a project") &&
		!strings.Contains(strings.ToLower(err.Error()), "did not resolve") &&
		!strings.Contains(strings.ToLower(err.Error()), "unable to resolve") {
		t.Fatalf("RunDoctor(unresolvable project) error = %v, want select/resolve guidance", err)
	}
	if called {
		t.Fatalf("scoped diagnostics ran for an unresolvable project, want refusal before any run")
	}
}

// I1 (legacy path): an empty project keeps the host-level working-directory
// run.
func TestHealthService_RunDoctorEmptyProjectRunsUnscoped(t *testing.T) {
	setupHealthServiceTest(t)
	called := false
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		called = true
		return stubDoctorServiceReport()
	}))

	svc := newHealthServiceForTest()
	report, err := svc.RunDoctor("")
	if err != nil {
		t.Fatalf("RunDoctor(empty project) error = %v, want the legacy host-level run", err)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("RunDoctor(empty project) returned %d checks, want 4", len(report.Checks))
	}
	if !called {
		t.Fatalf("legacy unscoped run never reached the diagnostics seam")
	}
}

// I1 (package API): RunDoctorForProject scopes the real engine to the root.
// A legacy config in the root warns while the suite working directory
// (which holds no .govard.yml) would not, proving the run read the root.
func TestRunDoctorForProjectAuditsTheRoot(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n  features:\n    redis: true\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy .govard.yml: %v", err)
	}

	report, err := desktop.RunDoctorForProject(root)
	if err != nil {
		t.Fatalf("RunDoctorForProject(root) error = %v", err)
	}
	for _, check := range report.Checks {
		if check.ID == "project.config.audit" {
			if check.Status != string(engine.DoctorStatusWarn) {
				t.Fatalf("project.config.audit status = %q, want warn (legacy keys in the scoped root)", check.Status)
			}
			return
		}
	}
	t.Fatalf("RunDoctorForProject(root) has no project.config.audit check, want a legacy-key warning from the scoped root")
}
