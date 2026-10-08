package tests

import (
	"context"
	"reflect"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// TestP5RestoreReceivesTheGatedSnapshotName pins issue #461: the restore item
// must pass the snapshot name as a positional argument. `govard snapshot restore`
// declares cobra.ExactArgs(1), so the old argument-less call failed validation
// and restored nothing while the checklist reported it as an item that ran.
func TestP5RestoreReceivesTheGatedSnapshotName(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	var restoreArgs []string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "restore" {
			restoreArgs = append([]string(nil), args...)
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	if _, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 5,
		verify.VerifyOpts{AllowDestructive: true, ProjectRoot: root}); err != nil {
		t.Fatalf("RunPhase: %v", err)
	}

	want := []string{"snapshot", "restore", "20260101-000000", "-y"}
	if !reflect.DeepEqual(restoreArgs, want) {
		t.Fatalf("P5-05 invoked %v, want %v", restoreArgs, want)
	}
}

// TestP5RestoreRefusesWithoutAGatedSnapshot pins the other half: with no
// snapshot the item must fail loudly instead of invoking a restore that cannot
// work. P5-02 has already deleted the volumes by then, so a silent pass here is
// indistinguishable from a working restore.
func TestP5RestoreRefusesWithoutAGatedSnapshot(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()

	restoreCalled := false
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "restore" {
			restoreCalled = true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	item, ok := findItem("P5-05")
	if !ok {
		t.Fatal("P5-05 is missing from the registry")
	}
	ev := item.Run(context.Background(), engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: root})

	if restoreCalled {
		t.Fatal("P5-05 invoked a restore with no gated snapshot")
	}
	if ev.ExitCode == 0 {
		t.Fatalf("P5-05 passed with no gated snapshot: %q", ev.OutputExcerpt)
	}
}

func findItem(id string) (verify.Item, bool) {
	for _, it := range verify.Registry {
		if it.ID == id {
			return it, true
		}
	}
	return verify.Item{}, false
}
