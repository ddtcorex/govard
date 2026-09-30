package tests

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// A real gzip stream with content: the gate requires a dump that can
	// actually restore, so a placeholder string would make every fixture
	// snapshot unusable.
	var dump bytes.Buffer
	zw := gzip.NewWriter(&dump)
	if _, err := zw.Write([]byte("-- MySQL dump\nCREATE TABLE `x` (id int);\n")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db.sql.gz"), dump.Bytes(), 0o644); err != nil {
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

// TestGateRejectsSnapshotWhoseDumpIsEmpty reproduces a `snapshot create` whose
// dump failed: it still exits 0 and prints SUCCESS, leaving a valid but EMPTY
// gzip plus metadata with db:false. The gate must not authorise the destructive
// phase on it — P5-02 deletes the volume and this snapshot restores nothing.
func TestGateRejectsSnapshotWhoseDumpIsEmpty(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(engine.SnapshotRoot(root), "20260101-000000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A valid gzip stream with no content, exactly what a failed dump leaves.
	var empty bytes.Buffer
	zw := gzip.NewWriter(&empty)
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db.sql.gz"), empty.Bytes(), 0o644); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	meta := "name: 20260101-000000\ncreated_at: 2026-01-01T00:00:00Z\ndb: false\nmedia: false\n"
	if err := os.WriteFile(filepath.Join(dir, "metadata.yml"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	if name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatalf("gate opened on a snapshot with an empty dump (%q); P5-02 would delete the volume and P5-05 would restore nothing", name)
	}
}

// TestGateRejectsMediaOnlySnapshot covers the other half of the same failure:
// media was copied, the dump was not, so the database still cannot be recovered.
func TestGateRejectsMediaOnlySnapshot(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(engine.SnapshotRoot(root), "20260101-000000")
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "x.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	meta := "name: 20260101-000000\ncreated_at: 2026-01-01T00:00:00Z\ndb: false\nmedia: true\n"
	if err := os.WriteFile(filepath.Join(dir, "metadata.yml"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	if name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatalf("gate opened on a media-only snapshot (%q); the database is what P5-02 destroys and media cannot bring it back", name)
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

// TestP4RecordsTheSnapshotItCreated pins the producer half of the gate contract:
// P4-08 must record the snapshot name in its evidence. Without it no phase-4
// artifact carries an artifact name, the gate can never open, and phase 5 becomes
// permanently unreachable while CI stays green.
func TestP4RecordsTheSnapshotItCreated(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()

	// The snapshot appears only when the row runs `snapshot create`, the way
	// the real command adds one to the store.
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "create" {
			makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	item, ok := findItem("P4-08")
	if !ok {
		t.Fatal("P4-08 is missing from the registry")
	}
	ev := item.Run(context.Background(), engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: root})

	if len(ev.Artifacts) != 1 || ev.Artifacts[0] != "20260101-000000" {
		t.Fatalf("P4-08 recorded artifacts %v, want [20260101-000000]", ev.Artifacts)
	}
}

// TestP4DoesNotRecordASnapshotItCannotRestore is the producer-side guard for the
// empty-dump trap: a snapshot whose dump failed must not be recorded as the
// phase-5 restore target.
func TestP4DoesNotRecordASnapshotItCannotRestore(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	// `snapshot create` exits 0 but leaves an empty dump behind.
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "create" {
			dir := filepath.Join(engine.SnapshotRoot(root), "20260101-000000")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			var empty bytes.Buffer
			zw := gzip.NewWriter(&empty)
			_ = zw.Close()
			if err := os.WriteFile(filepath.Join(dir, "db.sql.gz"), empty.Bytes(), 0o644); err != nil {
				t.Fatalf("write dump: %v", err)
			}
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	item, _ := findItem("P4-08")
	ev := item.Run(context.Background(), engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: root})

	if len(ev.Artifacts) != 0 {
		t.Fatalf("P4-08 recorded %v as a restore target despite an empty dump", ev.Artifacts)
	}
}

// TestGatePrefersTheNewestSatisfyingArtifact pins the scan order, which decides
// which snapshot P5-05 restores.
func TestGatePrefersTheNewestSatisfyingArtifact(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	makeSnapshot(t, root, "20260102-000000", "2026-01-02T00:00:00Z")

	dir := verify.ProjectRunsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	older := goodPhase4(root, "20260101-000000")
	newer := goodPhase4(root, "20260102-000000")
	for name, res := range map[string]verify.RunResult{
		"2026-01-01T00-00-00Z-phase4.json": older,
		"2026-01-02T00-00-00Z-phase4.json": newer,
	} {
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root})
	if !ok {
		t.Fatal("gate closed with two satisfiable artifacts, want open")
	}
	if name != "20260102-000000" {
		t.Fatalf("gate returned %q, want the newest artifact's snapshot %q", name, "20260102-000000")
	}
}

// TestGateFallsBackToAnOlderSatisfiableArtifact documents the intended fallback:
// a newer artifact whose snapshot is unusable must not block an older one that
// still names a restorable snapshot.
func TestGateFallsBackToAnOlderSatisfiableArtifact(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")

	dir := verify.ProjectRunsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	older := goodPhase4(root, "20260101-000000")
	newer := goodPhase4(root, "gone-20260102-000000") // recorded but never created
	for name, res := range map[string]verify.RunResult{
		"2026-01-01T00-00-00Z-phase4.json": older,
		"2026-01-02T00-00-00Z-phase4.json": newer,
	} {
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root})
	if !ok {
		t.Fatal("gate closed; an older artifact still names a restorable snapshot")
	}
	if name != "20260101-000000" {
		t.Fatalf("gate returned %q, want the older satisfiable snapshot", name)
	}
}

// fakeAllExec makes every verify item succeed without touching Docker and
// returns a counter of how many govard invocations were attempted.
func fakeAllExec(t *testing.T) *int {
	t.Helper()
	fakeProbeHTTP(t)
	calls := 0
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, opts verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		calls++
		// P4-08 records the snapshot its `snapshot create` added to the store,
		// so the fake adds one, the way the real command does.
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "create" && opts.ProjectRoot != "" {
			makeSnapshot(t, opts.ProjectRoot, fmt.Sprintf("created-%d", calls), "2026-02-01T00:00:00Z")
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return &calls
}

// The run artifact is what the phase-5 gate reads, so it cannot depend on a
// flag that only shapes stdout.
func TestPhase4WritesArtifactWithoutJSON(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	fakeAllExec(t)

	if _, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4,
		verify.VerifyOpts{JSON: false, ProjectRoot: root}); err != nil {
		t.Fatalf("RunPhase 4: %v", err)
	}
	entries, err := os.ReadDir(verify.ProjectRunsDir(root))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no run artifact written with JSON:false (err=%v, entries=%d)", err, len(entries))
	}
}

func TestPhase5GateSeesPhase4RunWithoutJSON(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	fakeAllExec(t)
	cfg := engine.Config{Framework: "magento2"}

	if _, err := verify.RunPhase(context.Background(), cfg, 4, verify.VerifyOpts{ProjectRoot: root}); err != nil {
		t.Fatalf("RunPhase 4: %v", err)
	}
	if name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); !ok || !strings.HasPrefix(name, "created-") {
		t.Fatalf("gate = (%q, %v) after a JSON:false phase 4, want the snapshot phase 4 created", name, ok)
	}
	if _, err := verify.RunPhase(context.Background(), cfg, 5,
		verify.VerifyOpts{ProjectRoot: root, AllowDestructive: true}); err != nil {
		t.Fatalf("RunPhase 5 after a JSON:false phase 4: %v", err)
	}
}

// An artifact that cannot be written is a warning: the verdict is the items.
func TestRunPhaseUnwritableRunsDirKeepsTheVerdict(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	// Occupy the runs dir path with a regular file so MkdirAll and WriteFile fail.
	dir := verify.ProjectRunsDir(root)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeAllExec(t)

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 1,
		verify.VerifyOpts{ProjectRoot: root})
	if err != nil {
		t.Fatalf("an unwritable runs dir changed RunPhase into an error: %v", err)
	}
	if res.Status != "passed" || res.Failed() {
		t.Fatalf("verdict = %q failed=%v, want passed: the artifact write must not decide it", res.Status, res.Failed())
	}
}

func TestRunPhaseZeroEnforcesDestructiveGate(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	calls := fakeAllExec(t)

	_, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 0,
		verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if !errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("RunPhase(0) = %v, want ErrNeedAllowDestructive", err)
	}
	if *calls != 0 {
		t.Fatalf("RunPhase(0) ran %d command(s) before refusing", *calls)
	}
}

// Phase 0 runs phase 5 inside one call, so a snapshot recorded by an earlier
// run must already exist: P5-05 reads it from the store, not from this call.
func TestRunPhaseZeroNeedsARecordedSnapshot(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	calls := fakeAllExec(t)

	_, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 0,
		verify.VerifyOpts{ProjectRoot: t.TempDir(), AllowDestructive: true})
	if !errors.Is(err, verify.ErrNeedSnapshot) {
		t.Fatalf("RunPhase(0) = %v, want ErrNeedSnapshot", err)
	}
	if *calls != 0 {
		t.Fatalf("RunPhase(0) ran %d command(s) before refusing", *calls)
	}
}

func TestPreflightPhaseSelection(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	all := []int{1, 2, 3, 4, 5}
	cases := []struct {
		name   string
		phases []int
		opts   verify.VerifyOpts
		want   error
	}{
		{"phase 1 needs no gate", []int{1}, verify.VerifyOpts{ProjectRoot: root}, nil},
		{"all phases without the flag", all, verify.VerifyOpts{ProjectRoot: root}, verify.ErrNeedAllowDestructive},
		{"all phases with the flag: phase 4 will record the snapshot", all, verify.VerifyOpts{ProjectRoot: root, AllowDestructive: true}, nil},
		{"phase 5 alone, flag, no snapshot", []int{5}, verify.VerifyOpts{ProjectRoot: root, AllowDestructive: true}, verify.ErrNeedSnapshot},
		{"phase 5 alone, no flag", []int{5}, verify.VerifyOpts{ProjectRoot: root}, verify.ErrNeedAllowDestructive},
		{"plan bypasses the gate", all, verify.VerifyOpts{ProjectRoot: root, Plan: true}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verify.PreflightPhaseSelection(tc.phases, tc.opts); !errors.Is(got, tc.want) {
				t.Fatalf("PreflightPhaseSelection = %v, want %v", got, tc.want)
			}
		})
	}
}
