package tests

import (
	"sync"
	"testing"

	"govard/internal/desktop"
)

// TestOperationRegistry_CancelFiresRegisteredFunc registers two ops and
// cancels one: the cancelled op's func must fire exactly once while the
// other op stays listed.
func TestOperationRegistry_CancelFiresRegisteredFunc(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	var fired int
	if !desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() { fired++ }) {
		t.Fatalf("RegisterOp(new id) = false, want true")
	}
	if !desktop.RegisterOp("pull:proj-b", "proj-b", "pull", func() {}) {
		t.Fatalf("RegisterOp(new id) = false, want true")
	}

	if !desktop.CancelOp("sync:proj-a") {
		t.Fatalf("CancelOp(known id) = false, want true")
	}
	if fired != 1 {
		t.Fatalf("cancel func fired %d times, want exactly 1", fired)
	}

	ops := desktop.ListOps()
	if len(ops) != 1 || ops[0].ID != "pull:proj-b" {
		t.Fatalf("ListOps after cancel = %+v, want only pull:proj-b", ops)
	}
	if ops[0].Project != "proj-b" || ops[0].Kind != "pull" {
		t.Fatalf("OpInfo metadata = %+v, want Project proj-b Kind pull", ops[0])
	}
	if ops[0].StartedAt.IsZero() {
		t.Fatalf("OpInfo.StartedAt is zero, want registry-stamped time")
	}
}

// TestOperationRegistry_CancelUnknownReturnsFalse ensures cancelling an id
// that was never registered reports failure so callers can distinguish it
// from a successful cancel.
func TestOperationRegistry_CancelUnknownReturnsFalse(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	if desktop.CancelOp("sync:no-such-project") {
		t.Fatalf("CancelOp(unknown id) = true, want false")
	}
}

// TestOperationRegistry_CancelAfterFinishIsHarmlessNoOp pins the review
// focus: pressing Cancel after the op already finished must be harmless
// (no panic, cancel func NOT fired again). The registry reports false
// because no running op exists; callers treat that as success and show no
// error toast.
func TestOperationRegistry_CancelAfterFinishIsHarmlessNoOp(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	var fired int
	if !desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() { fired++ }) {
		t.Fatalf("RegisterOp(new id) = false, want true")
	}

	if !desktop.FinishOp("sync:proj-a") {
		t.Fatalf("FinishOp(known id) = false, want true")
	}
	if desktop.CancelOp("sync:proj-a") {
		t.Fatalf("CancelOp after finish = true, want false (nothing running)")
	}
	if fired != 0 {
		t.Fatalf("cancel func fired %d times after finish, want 0", fired)
	}

	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after finish = %+v, want empty", got)
	}
}

// TestOperationRegistry_CancelFiresExactlyOnce ensures a double cancel of a
// running op fires the func once and reports false the second time.
func TestOperationRegistry_CancelFiresExactlyOnce(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	var fired int
	if !desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() { fired++ }) {
		t.Fatalf("RegisterOp(new id) = false, want true")
	}

	if !desktop.CancelOp("sync:proj-a") {
		t.Fatalf("first CancelOp = false, want true")
	}
	if desktop.CancelOp("sync:proj-a") {
		t.Fatalf("second CancelOp = true, want false (nothing running)")
	}
	if fired != 1 {
		t.Fatalf("cancel func fired %d times, want exactly 1", fired)
	}
}

// TestOperationRegistry_DuplicateRegisterKeepsOriginal ensures a second
// RegisterOp for a running id is rejected and the original entry is left
// untouched, so the displaced cancel func can never orphan its context.
func TestOperationRegistry_DuplicateRegisterKeepsOriginal(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	var first, second int
	if !desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() { first++ }) {
		t.Fatalf("RegisterOp(new id) = false, want true")
	}
	if desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() { second++ }) {
		t.Fatalf("RegisterOp(duplicate id) = true, want false")
	}

	if !desktop.CancelOp("sync:proj-a") {
		t.Fatalf("CancelOp after duplicate register = false, want true")
	}
	if first != 1 {
		t.Fatalf("original cancel func fired %d times, want exactly 1", first)
	}
	if second != 0 {
		t.Fatalf("displaced cancel func fired %d times, want 0 (never adopted)", second)
	}
}

// TestOperationRegistry_FinishCyclesDoNotGrowMap ensures repeated
// register/finish cycles leave no residue: the live map holds only running
// ops, finished ids are not retained.
func TestOperationRegistry_FinishCyclesDoNotGrowMap(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	for i := 0; i < 50; i++ {
		if !desktop.RegisterOp("sync:proj-a", "proj-a", "sync", func() {}) {
			t.Fatalf("RegisterOp cycle %d = false, want true", i)
		}
		if !desktop.FinishOp("sync:proj-a") {
			t.Fatalf("FinishOp cycle %d = false, want true", i)
		}
	}

	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after 50 finish cycles = %+v, want empty", got)
	}
	if desktop.CancelOp("sync:proj-a") {
		t.Fatalf("CancelOp after finish cycles = true, want false (nothing retained)")
	}
}

// TestOperationRegistry_ConcurrentAccess exercises the registry from
// multiple goroutines; the suite runs under -race so an unguarded map
// fails here.
func TestOperationRegistry_ConcurrentAccess(t *testing.T) {
	desktop.ResetOperationRegistryForTest()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			desktop.RegisterOp("op", "proj", "sync", func() {})
			desktop.ListOps()
			desktop.CancelOp("op")
		}()
	}
	wg.Wait()
}
