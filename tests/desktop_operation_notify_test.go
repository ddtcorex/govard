package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// The tray shows the running-op count: plain "Govard" when idle, with a
// "(1 op)/(N ops)" badge while operations run.
func TestDesktopTrayTitleShowsRunningOpCount(t *testing.T) {
	if got := desktop.TrayTitleForTest(0); got != "Govard" {
		t.Fatalf("tray title idle = %q, want %q", got, "Govard")
	}
	if got := desktop.TrayTitleForTest(1); got != "Govard (1 op)" {
		t.Fatalf("tray title 1 op = %q, want %q", got, "Govard (1 op)")
	}
	if got := desktop.TrayTitleForTest(3); got != "Govard (3 ops)" {
		t.Fatalf("tray title 3 ops = %q, want %q", got, "Govard (3 ops)")
	}
	if got := desktop.TrayTitleForTest(-1); got != "Govard" {
		t.Fatalf("tray title -1 ops = %q, want %q (clamped to idle)", got, "Govard")
	}
}

// A failure notification whose event names its exact source terminal record
// carries the catalog actions (Retry/Open-logs) plus that record id, which
// the Retry button replays through RetryOp.
func TestDesktopOperationNotificationCarriesRetryActionForFailedRecord(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusFailed, "connection refused") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "sync",
		Status:    engine.OperationStatusFailure,
		Project:   "proj-a",
		Message:   "connection refused",
		OpID:      "sync:proj-a",
	})
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if notification.RetryID != "sync:proj-a" {
		t.Fatalf("notification RetryID = %q, want %q", notification.RetryID, "sync:proj-a")
	}
	ids := make([]string, 0, len(notification.Actions))
	for _, action := range notification.Actions {
		ids = append(ids, action.ID)
	}
	if !containsNotifyAction(ids, "retry") || !containsNotifyAction(ids, "open-logs") {
		t.Fatalf("notification actions = %q, want Retry + Open-logs", ids)
	}
}

// A failure with no matching terminal record offers Open-logs but no Retry:
// there is nothing RetryOp could replay.
func TestDesktopOperationNotificationOmitsRetryWithoutFailedRecord(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "sync",
		Status:    engine.OperationStatusFailure,
		Project:   "proj-a",
		Message:   "connection refused",
	})
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if notification.RetryID != "" {
		t.Fatalf("notification RetryID = %q, want empty (no terminal record)", notification.RetryID)
	}
	ids := make([]string, 0, len(notification.Actions))
	for _, action := range notification.Actions {
		ids = append(ids, action.ID)
	}
	if containsNotifyAction(ids, "retry") {
		t.Fatalf("notification actions = %q, want no Retry without a record", ids)
	}
	if !containsNotifyAction(ids, "open-logs") {
		t.Fatalf("notification actions = %q, want Open-logs", ids)
	}
}

// Success notifications carry no actions: there is nothing to retry and no
// failure to inspect.
func TestDesktopOperationNotificationSuccessCarriesNoActions(t *testing.T) {
	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "up",
		Status:    engine.OperationStatusSuccess,
		Project:   "proj-a",
		Message:   "environment started",
	})
	if !ok {
		t.Fatal("expected success operation notification")
	}
	if len(notification.Actions) != 0 {
		t.Fatalf("success actions = %+v, want none", notification.Actions)
	}
	if notification.RetryID != "" {
		t.Fatalf("success RetryID = %q, want empty", notification.RetryID)
	}
}

// A missing cursor means the log rotated or the app was down: the gap is
// delivered, never silently dropped. (Fresh starts still suppress history:
// an empty cursor keeps the old behavior, see
// TestDesktopPkgSelectOperationEventsSinceForTest.)
func TestDesktopSelectOperationEventsDeliversGapOnMissingCursor(t *testing.T) {
	events := []engine.OperationEvent{
		{Timestamp: "2026-10-17T10:00:00Z", Operation: "up.run", Status: engine.OperationStatusSuccess},
		{Timestamp: "2026-10-17T10:01:00Z", Operation: "sync.run", Status: engine.OperationStatusFailure},
	}
	newEvents, nextCursor := desktop.SelectOperationEventsSinceForTest(events, "missing-cursor")
	if len(newEvents) != len(events) {
		t.Fatalf("missing cursor delivered %d events, want %d (the gap)", len(newEvents), len(events))
	}
	if nextCursor != desktop.OperationEventSignatureForTest(events[len(events)-1]) {
		t.Fatalf("cursor not advanced to latest event after gap delivery")
	}
}

// Restart keeps the notification cursor on disk: the downtime gap is
// delivered once, and already-seen events are never re-notified.
func TestDesktopOperationWatcherPersistsCursorAcrossRestart(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))

	const pollInterval = 100 * time.Millisecond
	writeEvent := func(operation, message string) {
		t.Helper()
		if err := engine.WriteOperationEvent(engine.OperationEvent{
			Operation: operation, Status: engine.OperationStatusSuccess, Project: "sample-project", Message: message,
		}); err != nil {
			t.Fatalf("write event %s: %v", operation, err)
		}
	}
	waitForNotifications := func(fake *desktop.FakePlatform, want int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for len(fake.EventsNamed("operations:notification")) < want {
			if time.Now().After(deadline) {
				t.Fatalf("emitted %d notifications, want %d",
					len(fake.EventsNamed("operations:notification")), want)
			}
			time.Sleep(pollInterval / 4)
		}
	}

	// Generation one: start the watcher, then write distinct events until
	// at least one notification lands. A fresh watcher suppresses whatever
	// its startup recovery sees first — possibly including early writes,
	// when the watcher goroutine loses the scheduling race — so retries
	// must be distinct events, and the count below must quiesce before the
	// restart: generation two (correctly) redelivers anything written but
	// not yet delivered, which would break its exactly-1 assertion.
	fakeOne := &desktop.FakePlatform{}
	appOne := desktop.NewApp(desktop.WithPlatform(fakeOne))
	desktop.SetOperationWatcherPollIntervalForTest(appOne, pollInterval)
	ctxOne, cancelOne := context.WithCancel(context.Background())
	desktop.StartOperationWatcherForTest(appOne, ctxOne)
	notificationCount := func() int {
		return len(fakeOne.EventsNamed("operations:notification"))
	}
	genOneDeadline := time.Now().Add(10 * time.Second)
	for writes := 0; notificationCount() == 0; writes++ {
		if time.Now().After(genOneDeadline) {
			cancelOne()
			t.Fatal("generation one emitted no notification")
		}
		writeEvent("gen-one-op", fmt.Sprintf("gen-one-message-%d", writes))
		time.Sleep(pollInterval)
	}
	quiesceDeadline := time.Now().Add(10 * time.Second)
	for stable, last := 0, -1; stable < 3; {
		if count := notificationCount(); count == last {
			stable++
		} else {
			stable, last = 0, count
		}
		if time.Now().After(quiesceDeadline) {
			cancelOne()
			t.Fatal("generation one notifications never quiesced")
		}
		time.Sleep(pollInterval)
	}
	desktop.StopOperationWatcherForTest(appOne)
	cancelOne()

	// The cursor must reach disk once events are delivered. stop() only
	// cancels the watcher goroutine without joining it, and production
	// persists after emitting in the same poll (at-least-once by design),
	// so the persist for the observed notification may still be in flight
	// here: wait for the file instead of asserting it mid-race.
	cursorDeadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(desktop.OperationNotifyCursorPathForTest()); err == nil {
			break
		} else if time.Now().After(cursorDeadline) {
			t.Fatalf("notify cursor not persisted: %v", err)
		}
		time.Sleep(pollInterval / 4)
	}

	// Downtime: one event while no watcher runs.
	writeEvent("downtime-op", "downtime-message")

	// Generation two: a fresh App (fresh in-memory cursor) over the same
	// state dir must deliver the downtime gap without repeating generation
	// one's events.
	fakeTwo := &desktop.FakePlatform{}
	appTwo := desktop.NewApp(desktop.WithPlatform(fakeTwo))
	desktop.SetOperationWatcherPollIntervalForTest(appTwo, pollInterval)
	ctxTwo, cancelTwo := context.WithCancel(context.Background())
	desktop.StartOperationWatcherForTest(appTwo, ctxTwo)
	defer desktop.StopOperationWatcherForTest(appTwo)
	defer cancelTwo()
	waitForNotifications(fakeTwo, 1)

	payloads := fakeTwo.EventsNamed("operations:notification")
	if len(payloads) != 1 {
		t.Fatalf("generation two emitted %d notifications, want exactly the 1 downtime event", len(payloads))
	}
	notification, ok := payloads[0].(desktop.OperationNotification)
	if !ok {
		t.Fatalf("notification payload = %T, want desktop.OperationNotification", payloads[0])
	}
	if notification.Operation != "downtime-op" {
		t.Fatalf("restart delivered operation %q, want the downtime gap %q", notification.Operation, "downtime-op")
	}
}

func containsNotifyAction(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// With two failed records on one project, the notification for the older
// one binds exactly that record: the Retry button must replay the run the
// notification is about, never the newest failure.
func TestDesktopOperationNotificationRetryTargetsExactRecord(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	finishFailed := func(id, message string) {
		t.Helper()
		if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
			t.Fatalf("RegisterOpWithArgs(%s) = false, want true", id)
		}
		if !desktop.FinishOpWithStatus(id, desktop.OpStatusFailed, message) {
			t.Fatalf("FinishOpWithStatus(%s) = false, want true", id)
		}
	}
	finishFailed("sync:proj-a:old", "failure-A")
	finishFailed("sync:proj-a:new", "failure-B")

	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "sync",
		Status:    engine.OperationStatusFailure,
		Project:   "proj-a",
		Message:   "failure-A",
		OpID:      "sync:proj-a:old",
	})
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if notification.RetryID != "sync:proj-a:old" {
		t.Fatalf("notification RetryID = %q, want the exact source record %q", notification.RetryID, "sync:proj-a:old")
	}
}

// A failed record on the project is never enough on its own: an event that
// names no record id offers no Retry, proving no newest-scan fallback.
func TestDesktopOperationNotificationOmitsRetryWithoutOpID(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusFailed, "connection refused") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "sync",
		Status:    engine.OperationStatusFailure,
		Project:   "proj-a",
		Message:   "connection refused",
	})
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if notification.RetryID != "" {
		t.Fatalf("notification RetryID = %q, want empty (event names no record)", notification.RetryID)
	}
	for _, action := range notification.Actions {
		if action.ID == "retry" {
			t.Fatalf("notification offers Retry without a bound record id")
		}
	}
}

// An id naming an unknown or a non-failed record binds nothing: the toast
// offers no Retry rather than replaying something else.
func TestDesktopOperationNotificationOmitsRetryForUnusableOpID(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusCompleted, "ok") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	for _, opID := range []string{"sync:no-such-record", "sync:proj-a"} {
		notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
			Timestamp: "2026-10-17T10:00:00Z",
			Operation: "sync",
			Status:    engine.OperationStatusFailure,
			Project:   "proj-a",
			Message:   "connection refused",
			OpID:      opID,
		})
		if !ok {
			t.Fatal("expected failure operation notification")
		}
		if notification.RetryID != "" {
			t.Fatalf("OpID %q bound RetryID %q, want empty", opID, notification.RetryID)
		}
		for _, action := range notification.Actions {
			if action.ID == "retry" {
				t.Fatalf("OpID %q offers Retry without a failed record", opID)
			}
		}
	}
}

// A retry without a bound record id fails closed as record unavailable,
// never by scanning for a substitute.
func TestDesktopRetryOpWithoutIDFailsClosedAsRecordUnavailable(t *testing.T) {
	if _, err := desktop.RetryOp(""); err == nil {
		t.Fatal("expected record-unavailable refusal for empty id")
	} else if got := err.Error(); !strings.Contains(got, "record unavailable") {
		t.Fatalf("empty-id retry error = %q, want record unavailable", got)
	}
}

// At-least-once ordering: during one poll the notification is emitted
// BEFORE the cursor is persisted. The probe observes the cursor file at
// emit time: it must still hold the old cursor, proving a crash in that
// window leaves redelivery state behind instead of a loss.
func TestDesktopNotifyThenPersistEmitsBeforePersisting(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))

	writeOperationLogEventForTest(t, engine.OperationEvent{
		Operation: "probe-op", Status: engine.OperationStatusSuccess, Project: "sample-project", Message: "probe",
	})
	cursorPath := desktop.OperationNotifyCursorPathForTest()
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o700); err != nil {
		t.Fatalf("create cursor dir: %v", err)
	}
	if err := os.WriteFile(cursorPath, []byte("stale-cursor\n"), 0o600); err != nil {
		t.Fatalf("plant stale cursor: %v", err)
	}

	var observedAtEmit []string
	probe := &emitOrderProbePlatform{
		FakePlatform: &desktop.FakePlatform{},
		onEmit: func() {
			data, err := os.ReadFile(cursorPath)
			if err != nil {
				t.Errorf("read cursor at emit: %v", err)
				return
			}
			observedAtEmit = append(observedAtEmit, strings.TrimSpace(string(data)))
		},
	}
	next := desktop.PollOperationNotificationsForTest(probe, "stale-cursor")

	if got := len(probe.EventsNamed("operations:notification")); got != 1 {
		t.Fatalf("poll emitted %d notifications, want 1", got)
	}
	if len(observedAtEmit) != 1 || observedAtEmit[0] != "stale-cursor" {
		t.Fatalf("cursor at emit = %q, want the old cursor (persist must follow notify)", observedAtEmit)
	}
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor after poll: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != next {
		t.Fatalf("cursor after poll = %q, want advanced cursor %q", got, next)
	}
}

// Crash after notify but before persist: the cursor file still holds the
// old cursor (the persist never landed), so the restart redelivers. The
// duplicate is accepted; the loss is not.
func TestDesktopCrashAfterNotifyRedeliversOnRestart(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))
	cursorPath := desktop.OperationNotifyCursorPathForTest()

	writeOperationLogEventForTest(t, engine.OperationEvent{
		Operation: "crash-op", Status: engine.OperationStatusSuccess, Project: "sample-project", Message: "crash",
	})

	first := &desktop.FakePlatform{}
	desktop.PollOperationNotificationsForTest(first, "stale-cursor")
	if got := len(first.EventsNamed("operations:notification")); got != 1 {
		t.Fatalf("first poll emitted %d notifications, want 1", got)
	}

	// The crash wiped the persist: rewind the cursor file to its pre-poll
	// content while the notification was already shown.
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o700); err != nil {
		t.Fatalf("create cursor dir: %v", err)
	}
	if err := os.WriteFile(cursorPath, []byte("stale-cursor\n"), 0o600); err != nil {
		t.Fatalf("rewind cursor (simulate crash before persist): %v", err)
	}

	second := &desktop.FakePlatform{}
	desktop.PollOperationNotificationsForTest(second, "stale-cursor")
	payloads := second.EventsNamed("operations:notification")
	if len(payloads) != 1 {
		t.Fatalf("restart emitted %d notifications, want the 1 redelivery", len(payloads))
	}
	notification, ok := payloads[0].(desktop.OperationNotification)
	if !ok {
		t.Fatalf("notification payload = %T, want desktop.OperationNotification", payloads[0])
	}
	if notification.Operation != "crash-op" {
		t.Fatalf("restart delivered operation %q, want redelivery of %q", notification.Operation, "crash-op")
	}
}

// Crash before notify: the event lands while no poll ever ran, so the
// persisted cursor predates it and the restart delivers it with no loss and
// no repeat of older events.
func TestDesktopCrashBeforeNotifyDeliversGapOnRestart(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))

	writeOperationLogEventForTest(t, engine.OperationEvent{
		Operation: "before-op", Status: engine.OperationStatusSuccess, Project: "sample-project", Message: "before",
	})
	before := &desktop.FakePlatform{}
	cursor := desktop.PollOperationNotificationsForTest(before, "stale-cursor")
	if got := len(before.EventsNamed("operations:notification")); got != 1 {
		t.Fatalf("baseline poll emitted %d notifications, want 1", got)
	}

	// Downtime: one event lands with no poll running (the crash).
	writeOperationLogEventForTest(t, engine.OperationEvent{
		Operation: "gap-op", Status: engine.OperationStatusSuccess, Project: "sample-project", Message: "gap",
	})

	after := &desktop.FakePlatform{}
	desktop.PollOperationNotificationsForTest(after, cursor)
	payloads := after.EventsNamed("operations:notification")
	if len(payloads) != 1 {
		t.Fatalf("restart emitted %d notifications, want exactly the 1 gap event", len(payloads))
	}
	notification, ok := payloads[0].(desktop.OperationNotification)
	if !ok {
		t.Fatalf("notification payload = %T, want desktop.OperationNotification", payloads[0])
	}
	if notification.Operation != "gap-op" {
		t.Fatalf("restart delivered operation %q, want the gap event %q", notification.Operation, "gap-op")
	}
}

// writeOperationLogEventForTest appends one event to the test operations
// log. Callers set GOVARD_OPERATIONS_LOG_PATH to an isolated file first.
func writeOperationLogEventForTest(t *testing.T, event engine.OperationEvent) {
	t.Helper()
	if err := engine.WriteOperationEvent(event); err != nil {
		t.Fatalf("write event %s: %v", event.Operation, err)
	}
}

// emitOrderProbePlatform records FakePlatform emissions while observing
// outside state (the cursor file) at emit time, pinning the
// notify-before-persist order.
type emitOrderProbePlatform struct {
	*desktop.FakePlatform
	onEmit func()
}

// Emit implements desktop.Platform, observing first so the observation
// provably precedes anything the poll does after emitting.
func (p *emitOrderProbePlatform) Emit(event string, data any) {
	if p.onEmit != nil {
		p.onEmit()
	}
	p.FakePlatform.Emit(event, data)
}
