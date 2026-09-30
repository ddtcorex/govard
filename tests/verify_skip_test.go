package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// A framework-gated item is not an absent item. An unmet When used to drop the
// row outright, so a phase silently shrank (measured: 15 rows -> 13) and the
// report could not say what it had not run. The run must keep the row, mark it
// skipped, name the reason, and leave the item unexecuted.
func TestVerifyRunnerReportsUnmetWhenAsSkipped(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	idx := -1
	for i := range verify.Registry {
		if verify.Registry[i].ID == "P3-13" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("registry has no P3-13; the fixture this test gates moved")
	}
	origWhen, origRun := verify.Registry[idx].When, verify.Registry[idx].Run
	runCalls := 0
	verify.Registry[idx].When = func(engine.Config) bool { return false }
	verify.Registry[idx].Run = func(context.Context, engine.Config, verify.VerifyOpts) verify.Evidence {
		runCalls++
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "a skipped item must not run"}
	}
	t.Cleanup(func() {
		verify.Registry[idx].When = origWhen
		verify.Registry[idx].Run = origRun
	})

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 3,
		verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}

	var row *verify.RunItem
	for i := range res.Items {
		if res.Items[i].ID == "P3-13" {
			row = &res.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("P3-13 left no row in the phase-3 report (%d rows); a gated item must be reported as skipped", len(res.Items))
	}
	if !row.Skipped {
		t.Fatalf("P3-13 row Skipped = false, want true: %+v", *row)
	}
	if row.SkipReason == "" {
		t.Fatal("P3-13 skip row has an empty SkipReason; the artifact must say why the item did not run")
	}
	if runCalls != 0 {
		t.Fatalf("the gated item ran %d time(s), want 0: a skip must not execute the item", runCalls)
	}

	// A skip belongs in neither bucket: passed + skipped must account for every
	// row. The phase runs on the fake exec, whose excerpt carries no audit
	// session id, so P3-15 skips for its own reason as well — the invariant is
	// the identity below, not a count that assumes this gated row is the only
	// one that skips.
	skipped := res.SkippedCount()
	if skipped < 1 {
		t.Fatalf("SkippedCount() = %d, want at least the gated P3-13", skipped)
	}
	passed, failed := res.Counts()
	if failed != 0 {
		t.Fatalf("Counts() failed = %d, want 0: a skip is never red", failed)
	}
	if want := len(res.Items) - skipped; passed != want {
		t.Fatalf("Counts() passed = %d over %d rows with %d skipped, want %d: a skip belongs in neither bucket", passed, len(res.Items), skipped, want)
	}
}

// A skip must never turn a phase red: it is a row that says "accounted for, not
// run", so neither Counts() bucket may claim it and the verdict stays green.
func TestSkippedItemKeepsThePhaseVerdictGreen(t *testing.T) {
	skipped := verify.Skip("no fixture")
	res := verify.RunResult{Items: []verify.RunItem{
		{ID: "P3-01", ExitCode: 0},
		{ID: "P3-13", ExitCode: skipped.ExitCode, Skipped: skipped.Skipped, SkipReason: skipped.SkipReason},
	}}

	if res.Failed() {
		t.Fatal("Failed() = true with one green row and one skip, want false")
	}
	res.RefreshStatus()
	if res.Status != "passed" {
		t.Fatalf("Status = %q, want %q", res.Status, "passed")
	}
	if got := res.SkippedCount(); got != 1 {
		t.Fatalf("SkippedCount() = %d, want 1", got)
	}
	passed, failed := res.Counts()
	if passed != 1 || failed != 0 {
		t.Fatalf("Counts() = %d/%d, want 1/0: a skip counts in neither bucket", passed, failed)
	}
}

// The failure mode a phase-by-phase Magento run cannot show: on a framework
// whose Magento items are gated out, the phase now reports MORE rows than the
// pre-change baseline, each extra one skipped. That growth is intended — the
// rows used to vanish — and the merge `internal/cmd/verify.go` builds for a
// whole run (append items, refresh the verdict once) must keep them.
func TestGatedItemsAreCountedInAMergedAllPhasesRun(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	cfg := engine.Config{Framework: "laravel"}
	opts := verify.VerifyOpts{ProjectRoot: t.TempDir()}

	// The pre-change baseline: the phase-3 rows the old filter emitted, i.e.
	// those whose When predicate holds on this framework.
	baseline := 0
	for _, it := range verify.RegistryFor(cfg) {
		if it.Phase != 3 {
			continue
		}
		if it.When != nil && !it.When(cfg) {
			continue
		}
		baseline++
	}

	gated, err := verify.RunPhase(context.Background(), cfg, 3, opts)
	if err != nil {
		t.Fatalf("RunPhase phase 3: %v", err)
	}
	if len(gated.Items) <= baseline {
		t.Fatalf("phase 3 reported %d rows, want more than the pre-change baseline %d: a gated item must be visible, not dropped", len(gated.Items), baseline)
	}
	if gated.Status != "passed" {
		t.Fatalf("phase 3 status = %q, want passed: only skip rows were added", gated.Status)
	}

	skipped := map[string]bool{}
	for _, it := range gated.Items {
		if !it.Skipped {
			continue
		}
		skipped[it.ID] = true
		if it.SkipReason == "" {
			t.Fatalf("skipped row %q has an empty SkipReason", it.ID)
		}
	}
	// Every row the old filter dropped is present as a skip. The total may be
	// larger, because an item can also skip for its own reason: under the fake
	// exec P3-15 is handed no audit session id and skips instead of reporting a
	// green from a child that never ran.
	dropped := 0
	gatedByFramework := map[string]bool{}
	for _, it := range verify.RegistryFor(cfg) {
		if it.Phase != 3 || it.When == nil || it.When(cfg) {
			continue
		}
		dropped++
		gatedByFramework[it.ID] = true
		if !skipped[it.ID] {
			t.Fatalf("phase-3 row %s was dropped by the old filter but is not a skipped row in the report", it.ID)
		}
	}
	if dropped == 0 {
		t.Fatal("no phase-3 row is gated on this framework: this test can no longer see the row growth it exists for")
	}

	// A skip that names the wrong cause is the one lie this field can still tell.
	// `When` is a framework predicate, so the gate that fired is the framework,
	// not the item's precondition text — which on a Magento item reads like a
	// prior step ("P2-01 up") and sends the operator looking for a broken
	// environment instead of a project on the wrong framework. Only the
	// framework-gated rows are checked: a row that skipped for a reason of its
	// own (no module, no audit session) already tells the truth.
	for _, it := range gated.Items {
		if !gatedByFramework[it.ID] {
			continue
		}
		if !strings.Contains(it.SkipReason, "framework") {
			t.Errorf("framework-gated row %s skipped with %q, want a reason naming the framework gate rather than its Requires text", it.ID, it.SkipReason)
		}
	}
	if row, ok := findRunItem(gated, "P3-01"); !ok {
		t.Fatal("P3-01 left no row in the phase-3 report")
	} else if !row.Skipped || !strings.Contains(row.SkipReason, "laravel") {
		t.Errorf("P3-01 row = skipped %v reason %q, want a reason naming the project's framework (laravel)", row.Skipped, row.SkipReason)
	}

	// The merge path for a whole run: copy the first phase, append the next
	// phase's items, then refresh the verdict once.
	head, err := verify.RunPhase(context.Background(), cfg, 1, opts)
	if err != nil {
		t.Fatalf("RunPhase phase 1: %v", err)
	}
	merged := head
	merged.Phase = "all"
	merged.Items = append(merged.Items, gated.Items...)
	merged.RefreshStatus()

	if merged.Status != "passed" {
		t.Fatalf("merged status = %q, want passed", merged.Status)
	}
	if got, want := len(merged.Items), len(head.Items)+len(gated.Items); got != want {
		t.Fatalf("merged items = %d, want %d: the skip rows did not survive the append", got, want)
	}
	// Phase 1 skips too now (P1-06 has no lock file here), so the merge owes
	// both phases' skips.
	if got, want := merged.SkippedCount(), len(skipped)+head.SkippedCount(); got != want {
		t.Fatalf("merged SkippedCount() = %d, want %d: the merge lost a skip row", got, want)
	}
}
