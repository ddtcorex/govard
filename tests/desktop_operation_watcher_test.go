package tests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// The tray refresh rebuilds the whole menu and reads every project's container
// state, so a burst of notifications must not trigger one rebuild per event:
// that work runs inside the watcher's own goroutine and would delay the events
// behind it.
func TestDesktopOperationWatcherRefreshesOncePerPoll(t *testing.T) {
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))

	if err := engine.WriteOperationEvent(engine.OperationEvent{
		Operation: "baseline", Status: engine.OperationStatusSuccess, Project: "sample-project",
	}); err != nil {
		t.Fatalf("seed baseline event: %v", err)
	}

	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))
	refreshes := 0
	desktop.SetOperationWatcherRefreshForTest(app, func() { refreshes++ })

	// The production cadence is two seconds. Shortening it keeps the assertion --
	// one refresh per poll, not one per event -- and drops the two and a half
	// seconds this test used to spend waiting for a ticker to fire.
	const pollInterval = 20 * time.Millisecond
	desktop.SetOperationWatcherPollIntervalForTest(app, pollInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	desktop.StartOperationWatcherForTest(app, ctx)
	defer desktop.StopOperationWatcherForTest(app)

	// Let the watcher read its starting cursor before new events arrive.
	time.Sleep(300 * time.Millisecond)

	for _, operation := range []string{"up", "sync", "deploy"} {
		if err := engine.WriteOperationEvent(engine.OperationEvent{
			Operation: operation, Status: engine.OperationStatusSuccess, Project: "sample-project",
		}); err != nil {
			t.Fatalf("write event %s: %v", operation, err)
		}
	}

	// Wait for the notifications instead of sleeping a guessed interval: the
	// subject of this test is the refresh count, and a slow machine should not
	// fail it just for being slow.
	deadline := time.Now().Add(10 * time.Second)
	for len(fake.EventsNamed("operations:notification")) < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("emitted %d notifications, want 3", len(fake.EventsNamed("operations:notification")))
		}
		time.Sleep(pollInterval)
	}

	// Let a poll already in flight settle, so a refresh that was about to happen
	// is counted before the assertion rather than quietly after it.
	time.Sleep(4 * pollInterval)

	if got := len(fake.EventsNamed("operations:notification")); got != 3 {
		t.Fatalf("emitted %d notifications, want 3", got)
	}
	if refreshes != 1 {
		t.Fatalf("tray refreshed %d times for one poll, want 1", refreshes)
	}
}
