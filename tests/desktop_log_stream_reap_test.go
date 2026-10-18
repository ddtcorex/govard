package tests

import (
	"os/exec"
	"testing"
	"time"

	"govard/internal/desktop"
)

// Stopping a log stream must reap the docker child: kill without wait
// leaves a zombie per stop/start cycle.
func TestDesktopPkgReapLogStreamProcessReapsKilledChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep binary unavailable")
	}

	done := make(chan struct{}, 2)
	done <- struct{}{}
	done <- struct{}{}
	desktop.ReapLogStreamProcessForTest(cmd, done)

	if cmd.ProcessState == nil {
		t.Fatal("stream process was killed but never reaped (zombie)")
	}
	// Killed by signal, not exited: the assertion that matters is that Wait
	// ran at all (ProcessState set), which is what reaps the zombie.
	if cmd.ProcessState.String() == "" {
		t.Fatal("expected a process state after reap, got empty")
	}
}

// The reaper must wait for the pipe scanners before Wait: Wait closes the
// parent pipe ends, and a scanner still reading gets "file already closed"
// instead of EOF.
func TestDesktopPkgReapLogStreamProcessDrainsScanners(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep binary unavailable")
	}

	done := make(chan struct{}, 2)
	reaped := make(chan struct{})
	go func() {
		desktop.ReapLogStreamProcessForTest(cmd, done)
		close(reaped)
	}()

	select {
	case <-reaped:
		t.Fatal("reaper returned while scanners were still draining")
	case <-time.After(300 * time.Millisecond):
	}

	done <- struct{}{}
	done <- struct{}{}
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		t.Fatal("reaper did not return after scanners drained")
	}
	if cmd.ProcessState == nil {
		t.Fatal("stream process was never reaped")
	}
}

// A scanner wedged by a pipe-holding grandchild must not wedge Stop: the
// drain is bounded and Wait still reaps.
func TestDesktopPkgReapLogStreamProcessBoundsStuckScanners(t *testing.T) {
	restore := desktop.SetLogStreamDrainTimeoutForTest(300 * time.Millisecond)
	defer restore()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep binary unavailable")
	}

	done := make(chan struct{}, 2)
	started := time.Now()
	desktop.ReapLogStreamProcessForTest(cmd, done)
	elapsed := time.Since(started)

	if cmd.ProcessState == nil {
		t.Fatal("stream process was never reaped")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("reaper wedged on stuck scanners: took %v", elapsed)
	}
}
