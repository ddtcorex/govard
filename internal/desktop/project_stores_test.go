package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/audit"
	"govard/internal/engine"
	"govard/internal/verify"
)

// The desktop binary never links the CLI package, so DeleteProject only sees
// the verify, audit and lint-cache stores when the desktop registers the
// resolver itself. This package does not import internal/cmd on purpose.
func TestDesktopCollectsVerifyAuditAndLintStoresOfAProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	projectID := audit.ProjectID(canonical, "")
	dirs := []string{
		filepath.Join(home, "verify-runs", verify.ProjectID(root)),
		filepath.Join(home, "audit", projectID),
		filepath.Join(audit.DefaultLintCacheRoot(home), audit.LintTargetID(projectID, "project", canonical)),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	art := engine.CollectProjectArtifacts("desk", root, nil)
	got := map[string]bool{}
	for _, d := range art.StoreDirs {
		got[d] = true
	}
	for _, d := range dirs {
		if !got[d] {
			t.Errorf("desktop delete would keep %s (collected %v)", d, art.StoreDirs)
		}
	}
}
