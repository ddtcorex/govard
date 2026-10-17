package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// I1 (service half): HealthService.RunDoctor takes the selected project and
// threads its resolved root into a scoped doctor run. The scoped run audits
// that root, so the report the UI buckets under the project actually
// describes the project, never the process working directory.
func TestHealthService_RunDoctorThreadsProjectRoot(t *testing.T) {
	setupHealthServiceTest(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write temp .govard.yml: %v", err)
	}
	var scoped []string
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForProjectForDesktopForTest(
		func(projectRoot string) engine.DoctorReport {
			scoped = append(scoped, projectRoot)
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
	if len(scoped) != 1 {
		t.Fatalf("scoped diagnostic runs = %d, want exactly 1 through the project-root seam", len(scoped))
	}
	if filepath.Clean(scoped[0]) != filepath.Clean(root) {
		t.Fatalf("scoped run root = %q, want the selected project's root %q", scoped[0], root)
	}
}

// I1 (misattribution guard): a non-empty project that resolves to no
// directory refuses with select guidance instead of auditing CWD and
// bucketing the host report under the project.
func TestHealthService_RunDoctorUnresolvableProjectRefused(t *testing.T) {
	setupHealthServiceTest(t)
	called := false
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForProjectForDesktopForTest(
		func(projectRoot string) engine.DoctorReport {
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
// run through the scoped seam with an empty root.
func TestHealthService_RunDoctorEmptyProjectRunsUnscoped(t *testing.T) {
	setupHealthServiceTest(t)
	var scoped []string
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForProjectForDesktopForTest(
		func(projectRoot string) engine.DoctorReport {
			scoped = append(scoped, projectRoot)
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
	if len(scoped) != 1 || scoped[0] != "" {
		t.Fatalf("scoped diagnostic roots = %q, want exactly one empty (unscoped) run", scoped)
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
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForProjectForDesktopForTest(nil))

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
