package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

// I1 (engine half): the diagnostics support a project-root-scoped run. The
// project-bound checks (config audit, drift) read from the given root,
// never from the process working directory. The suite runs with a working
// directory that holds no .govard.yml, so an unscoped run omits the audit
// row while a run scoped to a legacy root warns on it.
func TestRunDoctorDiagnosticsInRootScopesProjectChecks(t *testing.T) {
	legacyRoot := t.TempDir()
	legacy := "project_name: sample-project\nstack:\n  db: mariadb\n  features:\n    redis: true\n"
	if err := os.WriteFile(filepath.Join(legacyRoot, ".govard.yml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy .govard.yml: %v", err)
	}
	modernRoot := t.TempDir()
	modern := "project_name: sample-project\nstack:\n  services:\n    db: mariadb\n"
	if err := os.WriteFile(filepath.Join(modernRoot, ".govard.yml"), []byte(modern), 0o644); err != nil {
		t.Fatalf("write modern .govard.yml: %v", err)
	}

	stubDeps := func() engine.DoctorDependencies {
		return engine.DoctorDependencies{
			CheckDockerStatus:        func() error { return nil },
			CheckDockerComposePlugin: func() error { return nil },
			CheckPortAvailable:       func(port string) bool { return true },
			CheckDiskScratch:         func() error { return nil },
			CheckGovardHomeWritable:  func() error { return nil },
			CheckNetworkConnectivity: func() error { return nil },
			CheckSearchIndexBlock:    func() error { return nil },
			CheckSSHAgentStatus:      func() (string, error) { return "ok", nil },
			CheckComposeSpam:         func() error { return nil },
			CheckGovardRegistry:      func() error { return nil },
			CheckSystemDependencies:  func() []string { return nil },
			CheckRuntimeImages:       func() ([]string, error) { return nil, nil },
			CheckProfileSync:         func() error { return nil },
		}
	}

	auditStatus := func(report engine.DoctorReport) (engine.DoctorCheckStatus, bool) {
		for _, check := range report.Checks {
			if check.ID == "project.config.audit" {
				return check.Status, true
			}
		}
		return "", false
	}

	scoped := engine.RunDoctorDiagnosticsInRoot(stubDeps(), legacyRoot)
	status, found := auditStatus(scoped)
	if !found {
		t.Fatalf("scoped run to legacy root has no project.config.audit check, want a legacy-key warning")
	}
	if status != engine.DoctorStatusWarn {
		t.Fatalf("scoped project.config.audit status = %q, want warn (legacy keys in the scoped root)", status)
	}

	clean := engine.RunDoctorDiagnosticsInRoot(stubDeps(), modernRoot)
	status, found = auditStatus(clean)
	if !found {
		t.Fatalf("scoped run to modern root has no project.config.audit check, want a pass row")
	}
	if status != engine.DoctorStatusPass {
		t.Fatalf("scoped project.config.audit status = %q, want pass (no legacy keys in the scoped root)", status)
	}

	// Control case: the unscoped run reads the suite working directory,
	// which holds no .govard.yml, so it carries no audit row at all. The
	// legacy warning above therefore came from the scoped root, not CWD.
	unscoped := engine.RunDoctorDiagnostics(stubDeps())
	if _, found := auditStatus(unscoped); found {
		t.Fatalf("unscoped run has a project.config.audit check, want none (CWD holds no config)")
	}
}
