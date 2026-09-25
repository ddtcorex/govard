//go:build !windows

package tests

import (
	"bufio"
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"govard/internal/cmd"
)

// `govard desktop --dev` starts `pnpm dev`, which runs `sh -c vite`, which
// runs node. Killing only pnpm left Vite holding port 5173 (strictPort), so
// the next --dev run failed. Stopping must take the whole process tree.
func TestDesktopDevStopKillsTheWholeProcessTree(t *testing.T) {
	proc, err := cmd.StartDevProcessForTest("sh", "-c", "sleep 300 & echo $!; wait")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	line, err := bufio.NewReader(proc.Stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read grandchild pid: %v", err)
	}
	grandchild, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse grandchild pid %q: %v", line, err)
	}

	cmd.StopDevProcessForTest(proc.Cmd)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(grandchild, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(grandchild, syscall.SIGKILL)
	t.Fatalf("grandchild %d survived stopping the dev process", grandchild)
}

// A signal delivered to govard alone (an IDE Stop button, `kill -INT <pid>`, a
// supervisor) cancels the root command's context. The app command must then take
// its whole process group with it, or `go run` is left waiting, the window stays
// open and Vite keeps port 5173 (issue #423; terminal Ctrl-C is unaffected
// because the terminal signals the whole foreground group).
func TestDesktopDevAppStopsWhenTheCommandContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc, err := cmd.StartDevAppForTest(ctx, "sh", "-c", "sleep 300 & echo $!; wait")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	line, err := bufio.NewReader(proc.Stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read grandchild pid: %v", err)
	}
	grandchild, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse grandchild pid %q: %v", line, err)
	}

	cancel()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(grandchild, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(grandchild, syscall.SIGKILL)
	t.Fatalf("grandchild %d survived the command context being cancelled", grandchild)
}
