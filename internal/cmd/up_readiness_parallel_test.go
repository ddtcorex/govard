package cmd

import (
	"errors"
	"sync"
	"testing"
	"time"

	"govard/internal/engine"
)

// upReadinessTestConfig asks for the three checks that matter: php, redis, db.
func upReadinessTestConfig() engine.Config {
	return engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			Services:   engine.Services{DB: "mariadb", Cache: "redis"},
		},
	}
}

func stubUpReadiness(t *testing.T, probes func(container string, args []string) error, states func(container string) (string, error)) {
	t.Helper()

	originalProbe := upReadinessProbeRunner
	originalState := upContainerStateRunner
	originalSleep := upReadinessSleep
	originalStart := upReadinessContainerStart
	t.Cleanup(func() {
		upReadinessProbeRunner = originalProbe
		upContainerStateRunner = originalState
		upReadinessSleep = originalSleep
		upReadinessContainerStart = originalStart
	})

	upReadinessProbeRunner = probes
	upContainerStateRunner = states
	upReadinessSleep = func(time.Duration) {}
	upReadinessContainerStart = func(string) {}
}

// TestUpReadinessRunsChecksConcurrently proves the checks are not walked one at
// a time: the probe runner only answers once every check has entered it, so a
// sequential implementation times out on the first check.
func TestUpReadinessRunsChecksConcurrently(t *testing.T) {
	const wantChecks = 3

	var (
		mu     sync.Mutex
		seen   = map[string]bool{}
		allIn  = make(chan struct{})
		closed bool
	)
	stubUpReadiness(t, func(container string, _ []string) error {
		mu.Lock()
		seen[container] = true
		if len(seen) >= wantChecks && !closed {
			closed = true
			close(allIn)
		}
		mu.Unlock()

		select {
		case <-allIn:
			return nil
		case <-time.After(500 * time.Millisecond):
			return errors.New("barrier timeout: readiness checks did not run concurrently")
		}
	}, func(string) (string, error) {
		return "running|0|false||", nil
	})

	err := waitForUpRuntimeReadiness(t.TempDir(), upReadinessTestConfig(), nil, time.Second)
	if err != nil {
		t.Fatalf("waitForUpRuntimeReadiness: %v", err)
	}

	mu.Lock()
	got := len(seen)
	mu.Unlock()
	if got != wantChecks {
		t.Fatalf("%d checks reached the probe runner, want %d", got, wantChecks)
	}
}

// TestUpReadinessToleratesOneExitedInspection pins the tolerance a container
// that exits once on creation gets: it is kickstarted, and readiness continues
// instead of failing env up.
func TestUpReadinessToleratesOneExitedInspection(t *testing.T) {
	originalPollEvery := upReadinessStatePollEvery
	upReadinessStatePollEvery = 1
	t.Cleanup(func() { upReadinessStatePollEvery = originalPollEvery })

	var (
		mu         sync.Mutex
		exitedOnce = map[string]bool{}
		kickstarts int
	)
	stubUpReadiness(t, func(string, []string) error { return nil }, func(container string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if !exitedOnce[container] {
			exitedOnce[container] = true
			return "exited|1|false||", nil
		}
		return "running|0|false||", nil
	})
	upReadinessContainerStart = func(string) {
		mu.Lock()
		kickstarts++
		mu.Unlock()
	}

	if err := waitForUpRuntimeReadiness(t.TempDir(), upReadinessTestConfig(), nil, 30*time.Second); err != nil {
		t.Fatalf("waitForUpRuntimeReadiness returned %v, want the single exit tolerated", err)
	}
	if kickstarts != 3 {
		t.Fatalf("%d kickstarts, want one per check", kickstarts)
	}
}

// TestUpReadinessInspectsStateAtReducedCadence pins the churn reduction: one
// container state inspection is shared by several probe attempts, because the
// inspection forks a process and the answer changes on a scale of seconds.
func TestUpReadinessInspectsStateAtReducedCadence(t *testing.T) {
	var (
		mu          sync.Mutex
		inspections = map[string]int{}
	)
	stubUpReadiness(t, func(string, []string) error { return errors.New("not ready") }, func(container string) (string, error) {
		mu.Lock()
		inspections[container]++
		mu.Unlock()
		return "running|0|false||", nil
	})

	// A one second timeout at a 250ms interval is four attempts.
	if err := waitForUpRuntimeReadiness(t.TempDir(), upReadinessTestConfig(), nil, time.Second); err == nil {
		t.Fatal("readiness succeeded although nothing ever became ready")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(inspections) != 3 {
		t.Fatalf("inspected %d containers, want the three checks", len(inspections))
	}
	for container, count := range inspections {
		if count > 2 {
			t.Fatalf("%s was inspected %d times for four attempts, want at most two", container, count)
		}
		if count == 0 {
			t.Fatalf("%s was never inspected", container)
		}
	}
}
