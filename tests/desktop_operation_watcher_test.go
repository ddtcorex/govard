package tests

import (
	"context"
	"path/filepath"
	"sync/atomic"
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

	writeEvent := func(operation string) {
		t.Helper()
		if err := engine.WriteOperationEvent(engine.OperationEvent{
			Operation: operation, Status: engine.OperationStatusSuccess, Project: "sample-project",
		}); err != nil {
			t.Fatalf("write event %s: %v", operation, err)
		}
	}

	writeEvent("baseline")

	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))
	// The refresh hook runs on the watcher's own goroutine while this test reads
	// the count on the test goroutine, so the counter is atomic. A plain int here
	// is a genuine data race -- it predates the poll-interval change (the same
	// pattern is on master), but tightening the interval brought the two
	// goroutines into real overlap, and `go test -race` fails it.
	var refreshes atomic.Int64
	desktop.SetOperationWatcherRefreshForTest(app, func() { refreshes.Add(1) })

	// The production cadence is two seconds; the test does not need to pay it.
	// It must not go much below a few hundred milliseconds either -- see the
	// probe event below for why the margin is the point here, not the speed.
	const pollInterval = 300 * time.Millisecond
	desktop.SetOperationWatcherPollIntervalForTest(app, pollInterval)

	notifications := func() int { return len(fake.EventsNamed("operations:notification")) }
	waitForNotifications := func(want int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for notifications() < want {
			if time.Now().After(deadline) {
				t.Fatalf("emitted %d notifications, want %d", notifications(), want)
			}
			time.Sleep(pollInterval / 4)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	desktop.StartOperationWatcherForTest(app, ctx)
	defer desktop.StopOperationWatcherForTest(app)

	// A probe event pins down the tick phase instead of guessing at it.
	//
	// The refresh fires once per poll that saw new events, not once per burst, so
	// "three events, one refresh" only holds if all three land inside the same
	// interval. Nothing tells the test when a tick last happened -- the ticker
	// starts inside the watcher goroutine -- so a burst written on a guessed
	// schedule can straddle a boundary and make the *correct* implementation
	// report two refreshes.
	//
	// Observing a probe notification proves both halves at once: the watcher has
	// read its starting cursor, and a tick has just fired -- so a full interval
	// remains for the burst to land in. The first probe is written optimistically
	// and may be swallowed if the watcher goroutine has not read its cursor yet,
	// so the loop retries until one is actually seen. That also removes the fixed
	// sleep the old version needed for the same job: the wait is a condition now.
	probes := 0
	for {
		writeEvent("probe")
		probeDeadline := time.Now().Add(2 * pollInterval)
		for notifications() <= probes && time.Now().Before(probeDeadline) {
			time.Sleep(pollInterval / 20)
		}
		probes = notifications()
		if probes > 0 {
			break
		}
	}

	refreshesAfterProbe := refreshes.Load()
	notificationsAfterProbe := notifications()

	for _, operation := range []string{"up", "sync", "deploy"} {
		writeEvent(operation)
	}
	waitForNotifications(notificationsAfterProbe + 3)

	// The refresh runs in the same tick, just after the emissions, so give it a
	// moment to be counted. The next tick is a full interval away, so nothing
	// else can arrive inside this window.
	time.Sleep(pollInterval / 6)

	// Asserted as deltas from the probe, so the result does not depend on how
	// many probes it took to synchronise.
	if got := notifications() - notificationsAfterProbe; got != 3 {
		t.Fatalf("burst emitted %d notifications, want 3", got)
	}
	// One refresh for the tick that carried all three burst events. Two would
	// mean they straddled a boundary; three would mean the tray is being rebuilt
	// per event, which is the regression this guards.
	if got := refreshes.Load() - refreshesAfterProbe; got != 1 {
		t.Fatalf("tray refreshed %d times for one poll carrying 3 events, want 1", got)
	}
}
