package tests

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/desktop"
)

// TestOperationRetry_ReplaysStoredArgsThroughHandler pins the Task 3
// contract: retrying a failed op replays the stored terminal record's args
// through the original service path (the handler registered for the record's
// kind), not a shortcut, and reports the replayed run's id.
func TestOperationRetry_ReplaysStoredArgsThroughHandler(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	var gotProject string
	var gotArgs []string
	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, project string, args []string) (string, error) {
		gotProject = project
		gotArgs = append([]string{}, args...)
		return "sync:proj-a#retry-1", nil
	})

	stored := []string{"sync", "--remote", "origin"}
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", stored, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusFailed, "connection refused") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	newID, err := desktop.RetryOp("sync:proj-a")
	if err != nil {
		t.Fatalf("RetryOp(failed op) = %v, want success", err)
	}
	if newID != "sync:proj-a#retry-1" {
		t.Errorf("RetryOp id = %q, want the replayed run's id from the service path", newID)
	}
	if gotProject != desktop.CanonicalProjectIDForTest("proj-a") {
		t.Errorf("handler project = %q, want canonical %q", gotProject, desktop.CanonicalProjectIDForTest("proj-a"))
	}
	if len(gotArgs) != len(stored) {
		t.Fatalf("handler args = %q, want exact stored args %q", gotArgs, stored)
	}
	for i := range stored {
		if gotArgs[i] != stored[i] {
			t.Fatalf("handler args = %q, want exact stored args %q", gotArgs, stored)
		}
	}
}

// TestOperationRetry_RefusesNonFailedStatuses pins the review focus: only
// failed ops are retryable. Succeeded, cancelled, and interrupted records
// must be refused, and the service path must never run for them.
func TestOperationRetry_RefusesNonFailedStatuses(t *testing.T) {
	for _, status := range []desktop.OpStatus{
		desktop.OpStatusCompleted,
		desktop.OpStatusCancelled,
		desktop.OpStatusInterrupted,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			desktop.ResetOperationRegistryForTest()
			desktop.ResetOpRetryHandlersForTest()
			seedHistoryProjectForTest(t, "proj-a")

			called := false
			desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
				called = true
				return "sync:proj-a#retry-1", nil
			})

			id := "sync:proj-a"
			if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
				t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
			}
			if !desktop.FinishOpWithStatus(id, status, "done") {
				t.Fatalf("FinishOpWithStatus(known id) = false, want true")
			}

			if _, err := desktop.RetryOp(id); err == nil {
				t.Fatalf("RetryOp(%s op) = success, want refusal", status)
			} else if !strings.Contains(strings.ToLower(err.Error()), "only failed") {
				t.Errorf("RetryOp(%s op) error = %q, want it to say only failed ops retry", status, err.Error())
			}
			if called {
				t.Errorf("service path ran for a %s op, want it never to run", status)
			}
		})
	}
}

// TestOperationRetry_RefusesScrubbedArgs is the fail-closed enforcement
// deferred from Task 2: a failed record whose args carry a "***" scrub
// placeholder (the placeholder no longer authenticates) must be refused with
// an actionable error asking for a fresh run, never replayed, and the
// refusal must never print the args.
func TestOperationRetry_RefusesScrubbedArgs(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	called := false
	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		called = true
		return "sync:proj-a#retry-1", nil
	})

	id := "sync:proj-a"
	// A bare "***" element passes the scrubber through unchanged, so the
	// stored record carries the marker exactly as a secret-scrubbed record
	// would.
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync", "***"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	_, err := desktop.RetryOp(id)
	if err == nil {
		t.Fatalf("RetryOp(scrubbed failed op) = success, want fail-closed refusal")
	}
	lowered := strings.ToLower(err.Error())
	if !strings.Contains(lowered, "fresh") {
		t.Errorf("RetryOp(scrubbed) error = %q, want an actionable fresh-credentials error", err.Error())
	}
	if strings.Contains(err.Error(), "***") {
		t.Errorf("RetryOp(scrubbed) error = %q, want args never echoed", err.Error())
	}
	if called {
		t.Errorf("service path ran for scrubbed args, want it never to run")
	}
}

// TestOperationRetry_ScrubbedSecretValueNeverEchoed replays the realistic
// shape: raw secret args are scrubbed at register, and the retry refusal
// must contain neither the raw secret nor the placeholder.
func TestOperationRetry_ScrubbedSecretValueNeverEchoed(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		t.Errorf("service path ran for scrubbed args, want it never to run")
		return "", nil
	})

	id := "sync:proj-a"
	raw := []string{"sync", "--password", "s3cret-value"}
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", raw, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	_, err := desktop.RetryOp(id)
	if err == nil {
		t.Fatalf("RetryOp(secret-scrubbed failed op) = success, want fail-closed refusal")
	}
	if strings.Contains(err.Error(), "s3cret-value") {
		t.Errorf("RetryOp error = %q, want the raw secret never echoed", err.Error())
	}
	if strings.Contains(err.Error(), "***") {
		t.Errorf("RetryOp error = %q, want the scrub placeholder never echoed", err.Error())
	}
}

// TestOperationRetry_UnknownID ensures retrying an id with no terminal
// record fails instead of inventing a run.
func TestOperationRetry_UnknownID(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if _, err := desktop.RetryOp("sync:no-such-op"); err == nil {
		t.Fatalf("RetryOp(unknown id) = success, want an error")
	}
}

// TestOperationRetry_UnknownKind ensures a failed record whose kind has no
// registered service path fails with the kind named (a kind is not secret)
// instead of running a shortcut.
func TestOperationRetry_UnknownKind(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	id := "deploy:proj-a"
	if !desktop.RegisterOpWithArgs(id, "proj-a", "deploy", []string{"deploy"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	_, err := desktop.RetryOp(id)
	if err == nil {
		t.Fatalf("RetryOp(unregistered kind) = success, want an error")
	}
	if !strings.Contains(err.Error(), "deploy") {
		t.Errorf("RetryOp(unregistered kind) error = %q, want it to name the kind", err.Error())
	}
}

// TestOperationService_ListsCancelsAndReadsHistory pins the bound surface:
// the OperationService methods the bridge routes reach must list running
// ops, cancel the tracked run, and read history.
func TestOperationService_ListsCancelsAndReadsHistory(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	svc := desktop.NewOperationService()
	var fired int
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", nil, func() { fired++ }) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}

	ops := svc.ListOps()
	if len(ops) != 1 || ops[0].ID != "sync:proj-a" {
		t.Fatalf("ListOps = %+v, want the one running op", ops)
	}

	if cancelled := svc.CancelOp("sync:proj-a"); !cancelled {
		t.Fatalf("CancelOp(known id) = false, want true")
	}
	if fired != 1 {
		t.Fatalf("cancel func fired %d times, want exactly 1 (the tracked run)", fired)
	}

	history := svc.ListHistory(0)
	if len(history) != 1 || history[0].Status != desktop.OpStatusCancelled {
		t.Fatalf("ListHistory = %+v, want the one cancelled record", history)
	}
}

// TestOperationRetry_ConcurrentDoubleRetryDispatchesOnce pins the fix-round
// ruling: two concurrent retries of the same failed record must dispatch the
// service path exactly once; the loser is refused with an in-progress error,
// never a second run (a double-clicked Retry button must not start two runs).
func TestOperationRetry_ConcurrentDoubleRetryDispatchesOnce(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	var executions atomic.Int64
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		executions.Add(1)
		entered <- struct{}{}
		<-release
		return "sync:proj-a#retry-1", nil
	})

	id := "sync:proj-a"
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	type outcome struct {
		id  string
		err error
	}
	first := make(chan outcome, 1)
	go func() {
		newID, err := desktop.RetryOp(id)
		first <- outcome{newID, err}
	}()
	// The first retry is inside the handler: the second retry now races it.
	<-entered
	second := make(chan outcome, 1)
	go func() {
		newID, err := desktop.RetryOp(id)
		second <- outcome{newID, err}
	}()
	// Let a double dispatch land before releasing: without the in-progress
	// mark the second call would already be inside the handler too.
	time.Sleep(200 * time.Millisecond)
	close(release)

	gotFirst := <-first
	gotSecond := <-second
	if gotFirst.err != nil {
		t.Fatalf("first RetryOp = %v, want success", gotFirst.err)
	}
	if executions.Load() != 1 {
		t.Fatalf("handler executed %d times, want exactly 1 (no double dispatch)", executions.Load())
	}
	if gotSecond.err == nil {
		t.Fatalf("second RetryOp = success as %q, want an in-progress refusal", gotSecond.id)
	}
	if !strings.Contains(strings.ToLower(gotSecond.err.Error()), "in progress") {
		t.Errorf("second RetryOp error = %q, want it to say a retry is already in progress", gotSecond.err.Error())
	}
}

// TestOperationRetry_HandlerMayReenterRegistry pins the dispatch-site
// invariant: handlers run OUTSIDE the registry lock, so a service path that
// lists or registers ops like any first run never deadlocks the retry.
func TestOperationRetry_HandlerMayReenterRegistry(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, project string, args []string) (string, error) {
		// A first run lists and registers through the same package API the
		// replay must behave like; under the registry lock this deadlocks.
		_ = desktop.ListOps()
		replayed := "sync:proj-a#retry-1"
		if !desktop.RegisterOpWithArgs(replayed, project, "sync", args, func() {}) {
			return "", fmt.Errorf("handler could not register replayed run")
		}
		if !desktop.FinishOpWithStatus(replayed, desktop.OpStatusCompleted, "ok") {
			return "", fmt.Errorf("handler could not finish replayed run")
		}
		return replayed, nil
	})

	id := "sync:proj-a"
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	// A lock held across dispatch deadlocks instead of answering, so bound
	// the call: a timeout here is the deadlock made observable.
	type outcome struct {
		id  string
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		newID, err := desktop.RetryOp(id)
		done <- outcome{newID, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("RetryOp(reentrant handler) = %v, want success", got.err)
		}
		if got.id != "sync:proj-a#retry-1" {
			t.Errorf("RetryOp id = %q, want the replayed run's id", got.id)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("RetryOp did not answer in 5s; the handler deadlocked against the registry lock")
	}
}

// TestOperationRetry_PanickingHandlerClearsMarkAndPropagates pins the fix
// review ruling: a handler panic must not wedge the record. The unmark runs
// on every path (including panic), the panic itself propagates to the
// service-level recovery boundary instead of being swallowed, and a later
// retry is allowed again.
func TestOperationRetry_PanickingHandlerClearsMarkAndPropagates(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		panic("service path blew up")
	})

	id := "sync:proj-a"
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = desktop.RetryOp(id)
	}()
	if !panicked {
		t.Errorf("RetryOp(panicking handler) did not panic; the panic must propagate, not be swallowed")
	}

	// The mark must be gone: the same record retries cleanly again once a
	// working service path is registered.
	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		return "sync:proj-a#retry-2", nil
	})
	if _, err := desktop.RetryOp(id); err != nil {
		t.Errorf("RetryOp after a panicking handler = %v, want success (the in-progress mark must clear even on panic)", err)
	}
}

// TestOperationRetry_InProgressRefusalGuidesWaiting ensures the concurrent
// refusal tells the user what to do: wait for the current run to finish
// before retrying.
func TestOperationRetry_InProgressRefusalGuidesWaiting(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	desktop.ResetOpRetryHandlersForTest()
	seedHistoryProjectForTest(t, "proj-a")

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	desktop.RegisterOpRetryHandler("sync", func(_ context.Context, _ string, _ []string) (string, error) {
		entered <- struct{}{}
		<-release
		return "sync:proj-a#retry-1", nil
	})

	id := "sync:proj-a"
	if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, "boom") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	done := make(chan error, 1)
	go func() {
		_, err := desktop.RetryOp(id)
		done <- err
	}()
	<-entered
	_, err := desktop.RetryOp(id)
	close(release)
	if err == nil {
		t.Fatalf("concurrent RetryOp = success, want an in-progress refusal")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "wait") {
		t.Errorf("in-progress refusal = %q, want wait guidance for the current run", err.Error())
	}
	if firstErr := <-done; firstErr != nil {
		t.Errorf("first RetryOp = %v, want success", firstErr)
	}
}
