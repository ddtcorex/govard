package tests

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// TestOperationHistory_RecordsCompletedOpWithArgsAndDuration pins the core
// Task 2 contract: a finished op lands in history with its replay args and
// a faithful duration (EndedAt - StartedAt).
func TestOperationHistory_RecordsCompletedOpWithArgsAndDuration(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	args := []string{"sync", "--remote", "origin"}
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", args, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusCompleted, "done") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory after one finish = %d records, want 1", len(history))
	}
	got := history[0]
	if got.ID != "sync:proj-a" {
		t.Errorf("record ID = %q, want sync:proj-a", got.ID)
	}
	if want := desktop.CanonicalProjectIDForTest("proj-a"); got.Project != want {
		t.Errorf("record Project = %q, want canonical %q", got.Project, want)
	}
	if got.Kind != "sync" {
		t.Errorf("record Kind = %q, want sync", got.Kind)
	}
	if !slices.Equal(got.Args, args) {
		t.Errorf("record Args = %q, want %q", got.Args, args)
	}
	if got.Status != desktop.OpStatusCompleted {
		t.Errorf("record Status = %q, want completed", got.Status)
	}
	if got.Message != "done" {
		t.Errorf("record Message = %q, want done", got.Message)
	}
	if got.StartedAt.IsZero() || got.EndedAt.IsZero() {
		t.Fatalf("record timestamps zero: started=%v ended=%v", got.StartedAt, got.EndedAt)
	}
	if got.EndedAt.Before(got.StartedAt) {
		t.Errorf("record EndedAt %v before StartedAt %v", got.EndedAt, got.StartedAt)
	}
	if want := got.EndedAt.Sub(got.StartedAt).Milliseconds(); got.DurationMS != want {
		t.Errorf("record DurationMS = %d, want %d (EndedAt-StartedAt)", got.DurationMS, want)
	}
	if ops := desktop.ListOps(); len(ops) != 0 {
		t.Errorf("ListOps after finish = %+v, want empty (history keeps the record, registry does not)", ops)
	}
}

// TestOperationHistory_RecordsFailedOpWithSanitizedMessage ensures failure
// text with ANSI escapes and control bytes is stored sanitized.
func TestOperationHistory_RecordsFailedOpWithSanitizedMessage(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", nil, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	raw := "\x1b[31mboom\x1b[0m\x07"
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusFailed, raw) {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory after one failure = %d records, want 1", len(history))
	}
	got := history[0]
	if got.Status != desktop.OpStatusFailed {
		t.Errorf("record Status = %q, want failed", got.Status)
	}
	if strings.ContainsAny(got.Message, "\x1b\x07") {
		t.Errorf("record Message = %q, want ANSI/control bytes stripped", got.Message)
	}
	if got.Message != "boom" {
		t.Errorf("record Message = %q, want boom", got.Message)
	}
}

// TestOperationHistory_CancelRecordsCancelledTerminal ensures CancelOp
// leaves exactly one cancelled terminal record (append terminal states).
func TestOperationHistory_CancelRecordsCancelledTerminal(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	var fired int
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", nil, func() { fired++ }) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.CancelOp("sync:proj-a") {
		t.Fatalf("CancelOp(known id) = false, want true")
	}
	if fired != 1 {
		t.Fatalf("cancel func fired %d times, want exactly 1", fired)
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory after cancel = %d records, want 1", len(history))
	}
	if history[0].Status != desktop.OpStatusCancelled {
		t.Errorf("record Status = %q, want cancelled", history[0].Status)
	}
}

// TestOperationHistory_CapEvictsOldestKeepsNewest drives 205 terminal ops
// through the public API and expects the 200 cap to evict the oldest five
// while keeping the newest intact, newest-first.
func TestOperationHistory_CapEvictsOldestKeepsNewest(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	const total = 205
	for i := 1; i <= total; i++ {
		id := fmt.Sprintf("op-%03d", i)
		if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
			t.Fatalf("RegisterOpWithArgs(%s) = false, want true", id)
		}
		if !desktop.FinishOpWithStatus(id, desktop.OpStatusCompleted, "") {
			t.Fatalf("FinishOpWithStatus(%s) = false, want true", id)
		}
	}

	history := desktop.ListHistory(0)
	if len(history) != desktop.OperationHistoryCap {
		t.Fatalf("ListHistory after %d finishes = %d records, want cap %d", total, len(history), desktop.OperationHistoryCap)
	}
	if history[0].ID != "op-205" {
		t.Errorf("newest record = %q, want op-205 (newest-first)", history[0].ID)
	}
	if history[len(history)-1].ID != "op-006" {
		t.Errorf("oldest kept record = %q, want op-006", history[len(history)-1].ID)
	}
	for _, id := range []string{"op-001", "op-005"} {
		for _, record := range history {
			if record.ID == id {
				t.Errorf("evicted record %q still present, want oldest five gone", id)
			}
		}
	}

	limited := desktop.ListHistory(2)
	if len(limited) != 2 || limited[0].ID != "op-205" || limited[1].ID != "op-204" {
		t.Errorf("ListHistory(2) = %v, want [op-205 op-204]", limitedIDs(limited))
	}
}

// TestOperationHistory_RestartMarksInFlightInterrupted simulates an app
// restart mid-sync: the in-flight op never finished, so recovery must mark
// it interrupted (never silent), preserve its args, and never duplicate it.
func TestOperationHistory_RestartMarksInFlightInterrupted(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	args := []string{"sync", "--remote", "origin"}
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", args, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}

	// Restart: running state is gone, the on-disk in-flight set remains.
	desktop.ClearOperationRegistryMemoryForTest()

	if got := desktop.RecoverInterruptedOps(); got != 1 {
		t.Fatalf("RecoverInterruptedOps = %d, want 1 orphan marked", got)
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory after recovery = %d records, want 1 (never silent)", len(history))
	}
	record := history[0]
	if record.Status != desktop.OpStatusInterrupted {
		t.Errorf("record Status = %q, want interrupted", record.Status)
	}
	if record.ID != "sync:proj-a" {
		t.Errorf("record ID = %q, want sync:proj-a", record.ID)
	}
	if !slices.Equal(record.Args, args) {
		t.Errorf("record Args = %q, want %q (kept for re-run)", record.Args, args)
	}

	if got := desktop.RecoverInterruptedOps(); got != 0 {
		t.Errorf("second RecoverInterruptedOps = %d, want 0 (no duplicates)", got)
	}
	if history := desktop.ListHistory(0); len(history) != 1 {
		t.Errorf("ListHistory after second recovery = %d records, want still 1", len(history))
	}
}

// seedHistoryProjectForTest registers name in an isolated project registry
// so canonicalProjectID resolves at its fast exact-match branch. Without a
// registry entry every RegisterOp would fall through to the orphan/Docker
// probes and one history test would shell out to `docker compose ls`
// hundreds of times.
func seedHistoryProjectForTest(t *testing.T, name string) {
	t.Helper()
	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, name, "laravel", name+".test")
	registerDesktopProjectForTest(t, root, name, name+".test", "laravel")
}

func limitedIDs(records []desktop.OpRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}
