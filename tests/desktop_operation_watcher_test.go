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

	// One poll interval plus slack.
	time.Sleep(2600 * time.Millisecond)

	if got := len(fake.EventsNamed("operations:notification")); got != 3 {
		t.Fatalf("emitted %d notifications, want 3", got)
	}
	if refreshes != 1 {
		t.Fatalf("tray refreshed %d times for one poll, want 1", refreshes)
	}
}
