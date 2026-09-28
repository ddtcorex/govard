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

// writePhase4 writes a phase-4 artifact into the scoped store.
func writePhase4(t *testing.T, root string, res verify.RunResult) {
	t.Helper()
	dir := verify.ProjectRunsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-01-01T00-00-00Z-phase4.json"), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// makeSnapshot creates a snapshot the way `govard snapshot create` does: a
// metadata.yml carrying a real created_at, plus a DB dump. ListSnapshots treats
// any directory as a snapshot and defaults CreatedAt to the zero time, so a
// bare directory is NOT a usable restore target and must not open the gate.
func makeSnapshot(t *testing.T, root, name, createdAt string) {
	t.Helper()
	dir := filepath.Join(engine.SnapshotRoot(root), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	meta := "name: " + name + "\ncreated_at: " + createdAt + "\ndb: true\nmedia: true\n"
	if err := os.WriteFile(filepath.Join(dir, "metadata.yml"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db.sql.gz"), []byte("not-a-real-dump"), 0o644); err != nil {
		t.Fatalf("write dump: %v", err)
	}
}

func goodPhase4(root, snapshot string) verify.RunResult {
	return verify.RunResult{
		Phase:     "phase4",
		ProjectID: verify.ProjectID(root),
		Mode:      "run",
		Items: []verify.RunItem{
			{ID: "P4-08", ExitCode: 0, Artifacts: []string{snapshot}},
		},
	}
}

func TestGateAcceptsOwnRunAndReturnsItsSnapshot(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root})
	if !ok {
		t.Fatal("gate closed for its own run with a snapshot on disk, want open")
	}
	if name != "20260101-000000" {
		t.Fatalf("gate returned %q, want the recorded snapshot %q", name, "20260101-000000")
	}
}

func TestGateRejectsPlanArtifact(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	res := goodPhase4(root, "20260101-000000")
	res.Mode = "plan"
	writePhase4(t, root, res)

	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("gate opened on a --plan artifact, want closed")
	}
}

func TestGateRejectsForeignProject(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root, other := t.TempDir(), t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	res := goodPhase4(root, "20260101-000000")
	res.ProjectID = verify.ProjectID(other)
	writePhase4(t, root, res)

	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("gate opened on another project's artifact, want closed")
	}
}

func TestGateRejectsSnapshotMissingFromDisk(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	writePhase4(t, root, goodPhase4(root, "20260101-000000")) // recorded but never created

	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("gate opened on a recorded snapshot that is not on disk, want closed")
	}
}

func TestGateRejectsUnusableSnapshotDirectory(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	// A bare directory with no metadata.yml and no dump: ListSnapshots still
	// reports it, but it cannot restore anything, so it must not open the gate.
	if err := os.MkdirAll(filepath.Join(engine.SnapshotRoot(root), "20260101-000000"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("gate opened on an empty snapshot directory, want closed")
	}
}

func TestGateRejectsLegacyArtifactWithoutIdentity(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	// An artifact from before project scoping: no project_id, no mode, no artifacts.
	legacy := verify.RunResult{Phase: "phase4", Items: []verify.RunItem{{ID: "P4-08", ExitCode: 0, EvidenceExcerpt: "ok"}}}
	writePhase4(t, root, legacy)

	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("gate opened on a legacy artifact with no project identity, want closed")
	}
}

func TestGateRejectsUnrecordedSnapshotName(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	// The recorded name is usable, but a DIFFERENT (newer) snapshot exists. The
	// gate must return the recorded one, not simply the newest on disk.
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	makeSnapshot(t, root, "20260102-000000", "2026-01-02T00:00:00Z")
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root})
	if !ok {
		t.Fatal("gate closed with a valid recorded snapshot, want open")
	}
	if name != "20260101-000000" {
		t.Fatalf("gate returned %q, want the RECORDED snapshot, not the newest", name)
	}
}

func TestLatestSnapshotNamePicksNewestUsable(t *testing.T) {
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	makeSnapshot(t, root, "20260102-000000", "2026-01-02T00:00:00Z")

	name, ok := verify.LatestSnapshotNameForTest(root)
	if !ok {
		t.Fatal("LatestSnapshotName found nothing, want the newest usable snapshot")
	}
	if name != "20260102-000000" {
		t.Fatalf("LatestSnapshotName = %q, want %q", name, "20260102-000000")
	}
}

func TestLatestSnapshotNameEmptyStoreIsNoSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(engine.SnapshotRoot(root), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if name, ok := verify.LatestSnapshotNameForTest(root); ok {
		t.Fatalf("empty store returned %q, want no snapshot", name)
	}
	if name, ok := verify.LatestSnapshotNameForTest(t.TempDir()); ok {
		t.Fatalf("missing store returned %q, want no snapshot", name)
	}
}

func TestLatestSnapshotNameIgnoresUnusableDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(engine.SnapshotRoot(root), "corrupt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if name, ok := verify.LatestSnapshotNameForTest(root); ok {
		t.Fatalf("returned the unusable directory %q, want no snapshot", name)
	}
}

func TestPlanPhase5RunsNothingAndStaysOutOfTheGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	root := t.TempDir()

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 5,
		verify.VerifyOpts{Plan: true, JSON: true, ProjectRoot: root, AllowDestructive: true})
	if err != nil {
		t.Fatalf("RunPhase plan: %v", err)
	}
	for _, it := range res.Items {
		if it.EvidenceExcerpt != "plan: "+it.Command {
			t.Fatalf("plan item %s executed: %q", it.ID, it.EvidenceExcerpt)
		}
	}
	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatal("a --plan phase-5 run satisfied the gate, want closed")
	}
}
