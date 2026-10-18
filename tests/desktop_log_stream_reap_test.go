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

	started := time.Now()
	desktop.ReapLogStreamProcessForTest(cmd)
	elapsed := time.Since(started)

	if cmd.ProcessState == nil {
		t.Fatal("stream process was killed but never reaped (zombie)")
	}
	// Killed by signal, not exited: the assertion that matters is that Wait
	// ran at all (ProcessState set), which is what reaps the zombie.
	if cmd.ProcessState.String() == "" {
		t.Fatal("expected a process state after reap, got empty")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("reap blocked: took %v", elapsed)
	}
}
