package tests

import (
	"reflect"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func TestDesktopPkgBuildOperationNotificationForTest(t *testing.T) {
	successEvent := engine.OperationEvent{
		Timestamp: "2026-02-20T03:00:00Z",
		Operation: "up.run",
		Status:    engine.OperationStatusSuccess,
		Project:   "demo",
		Message:   "environment started",
	}
	notification, ok := desktop.BuildOperationNotificationForTest(successEvent)
	if !ok {
		t.Fatal("expected success operation notification")
	}
	if notification.Level != "success" {
		t.Fatalf("expected success level, got %s", notification.Level)
	}
	if notification.Title != "Completed: up.run (demo)" {
		t.Fatalf("unexpected success title: %s", notification.Title)
	}
	if notification.Body != "environment started" {
		t.Fatalf("unexpected success body: %s", notification.Body)
	}

	failureEvent := engine.OperationEvent{
		Timestamp: "2026-02-20T03:05:00Z",
		Operation: "sync.run",
		Status:    engine.OperationStatusFailure,
		Message:   "ssh denied",
	}
	failureNotification, ok := desktop.BuildOperationNotificationForTest(failureEvent)
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if failureNotification.Level != "error" {
		t.Fatalf("expected error level, got %s", failureNotification.Level)
	}
	if failureNotification.Title != "Failed: sync.run" {
		t.Fatalf("unexpected failure title: %s", failureNotification.Title)
	}

	_, ok = desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Operation: "sync.run",
		Status:    engine.OperationStatusStart,
	})
	if ok {
		t.Fatal("did not expect notification for start status")
	}
}

func TestDesktopPkgSelectOperationEventsSinceForTest(t *testing.T) {
	events := []engine.OperationEvent{
		{Timestamp: "2026-02-20T03:00:00Z", Operation: "init.run", Status: engine.OperationStatusSuccess},
		{Timestamp: "2026-02-20T03:01:00Z", Operation: "up.run", Status: engine.OperationStatusSuccess},
	}

	newEvents, cursor := desktop.SelectOperationEventsSinceForTest(events, "")
	if len(newEvents) != 0 {
		t.Fatalf("expected empty initial event batch, got %d", len(newEvents))
	}
	expectedCursor := desktop.OperationEventSignatureForTest(events[len(events)-1])
	if cursor != expectedCursor {
		t.Fatalf("expected cursor %q, got %q", expectedCursor, cursor)
	}

	expanded := append(events, engine.OperationEvent{
		Timestamp: "2026-02-20T03:02:00Z",
		Operation: "sync.run",
		Status:    engine.OperationStatusFailure,
	})
	newEvents, cursor = desktop.SelectOperationEventsSinceForTest(expanded, cursor)
	if len(newEvents) != 1 {
		t.Fatalf("expected 1 new event, got %d", len(newEvents))
	}
	if !reflect.DeepEqual(newEvents[0], expanded[2]) {
		t.Fatalf("unexpected new event payload: %#v", newEvents[0])
	}
	if cursor != desktop.OperationEventSignatureForTest(expanded[len(expanded)-1]) {
		t.Fatalf("cursor not advanced to latest event")
	}

	rotated := []engine.OperationEvent{
		{Timestamp: "2026-02-20T04:00:00Z", Operation: "remote.test", Status: engine.OperationStatusSuccess},
	}
	newEvents, nextCursor := desktop.SelectOperationEventsSinceForTest(rotated, "missing-cursor")
	// A missing cursor is a gap (rotation, downtime), never a reason to go
	// silent: the window is delivered rather than dropped.
	if len(newEvents) != len(rotated) {
		t.Fatalf("expected gap delivery on missing cursor, got %d events", len(newEvents))
	}
	if nextCursor != desktop.OperationEventSignatureForTest(rotated[0]) {
		t.Fatalf("expected cursor to advance to latest after gap delivery, got %q", nextCursor)
	}
}
