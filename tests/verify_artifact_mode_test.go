package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// Run artifacts hold evidence excerpts, so the per-project directory is
// owner-only and so is every file written into it, even under a permissive umask.
func TestVerifyRunArtifactsAreOwnerOnly(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")
	t.Setenv(verify.EnvBinaryOverride, "")
	setPermissiveUmask(t)
	root := t.TempDir()

	if _, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 1,
		verify.VerifyOpts{JSON: true, ProjectRoot: root}); err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	dir := verify.ProjectRunsDir(root)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("runs dir mode = %o, want 700", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no artifact written: %v", err)
	}
	for _, entry := range entries {
		fi, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", entry.Name(), got)
		}
	}
}
