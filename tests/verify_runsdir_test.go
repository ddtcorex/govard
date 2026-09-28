package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

func TestProjectIDCanonicalisesPath(t *testing.T) {
	dir := t.TempDir()
	// A symlink to dir must resolve to the same id as dir itself.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	want := verify.ProjectID(dir)
	for _, variant := range []string{dir + string(os.PathSeparator), filepath.Join(dir, "."), link} {
		if got := verify.ProjectID(variant); got != want {
			t.Fatalf("ProjectID(%q) = %q, want %q (same project)", variant, got, want)
		}
	}
}

func TestProjectRunsDirScopedByProject(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	a, b := t.TempDir(), t.TempDir()
	if verify.ProjectRunsDir(a) == verify.ProjectRunsDir(b) {
		t.Fatal("two projects share one run store, want distinct directories")
	}
	if got, want := filepath.Dir(verify.ProjectRunsDir(a)), verify.VerifyRunsDir(); got != want {
		t.Fatalf("ProjectRunsDir parent = %q, want the store root %q", got, want)
	}
}

func TestResolveProjectSHANonRepoIsUnknown(t *testing.T) {
	// A directory that is not a git repository must degrade, not fail. Do NOT
	// assert on an empty root here: an empty root means the current working
	// directory, which in this repo IS a git checkout and would resolve.
	if got := verify.ResolveProjectSHAForTest(t.TempDir()); got != "unknown" {
		t.Fatalf("sha = %q, want \"unknown\"", got)
	}
}

func TestGOVARDHomeUnsetResolvesUnderUserHome(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home: %v", err)
	}
	if got, want := verify.ProjectRunsDir("/tmp/x"), filepath.Join(home, ".govard", "verify-runs", verify.ProjectID("/tmp/x")); got != want {
		t.Fatalf("ProjectRunsDir = %q, want %q", got, want)
	}
}

func TestRunPhaseWritesScopedArtifactWithIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_VERIFY_FAKE", "1")
	root := t.TempDir()

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 1,
		verify.VerifyOpts{JSON: true, ProjectRoot: root})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if res.ProjectID != verify.ProjectID(root) {
		t.Fatalf("artifact project_id = %q, want %q", res.ProjectID, verify.ProjectID(root))
	}
	if res.Mode != "run" {
		t.Fatalf("artifact mode = %q, want \"run\"", res.Mode)
	}
	entries, err := os.ReadDir(verify.ProjectRunsDir(root))
	if err != nil {
		t.Fatalf("read scoped store: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no artifact written under the scoped store")
	}
	b, err := os.ReadFile(filepath.Join(verify.ProjectRunsDir(root), entries[0].Name()))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var got verify.RunResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("artifact json: %v", err)
	}
	if got.ProjectID != verify.ProjectID(root) || got.Mode != "run" {
		t.Fatalf("artifact lost identity: %+v", got)
	}
}
