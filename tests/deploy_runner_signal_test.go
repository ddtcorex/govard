//go:build unix

package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"govard/internal/deploy"
)

// Every deploy step is a chain — `cd {{release_path}} && composer install …` —
// so the process govard starts is a shell and the work is that shell's child.
// Cancelling the context killed only the shell, which left the work running,
// unsupervised and still writing into the target, after the operator believed
// the deploy had stopped. This asserts on the work itself, not on the shell.
//
// The child ignores SIGTERM on purpose: `trap "" TERM` is inherited across
// exec, so a teardown that only sends SIGTERM leaves it running — exactly the
// shape of a long `composer install` or `rsync` that keeps mutating the target.
// Only the process group plus a final kill reaches it, which is why the
// assertion is what it is.
func TestAnInterruptedLocalCommandTakesItsChildrenWithIt(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "work.pid")

	// `exec` makes the recorded pid the work itself rather than the shell that
	// started it, so "is the work still running" is one process lookup and not a
	// guess about which of two pids to watch.
	command := "sh -c 'trap \"\" TERM; echo $$ > " + pidFile + "; exec sleep 60'"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := deploy.LocalRunner{}.Run(ctx, command, deploy.RunOptions{})
		done <- err
	}()

	pid := waitForRecordedPID(t, pidFile)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an interrupted command must fail")
		}
		if !strings.Contains(err.Error(), "interrupted") {
			t.Fatalf("the failure must say the run was interrupted, got %q", err.Error())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the interrupted command never returned")
	}

	waitForWorkToStop(t, pid, "local command's work")
}

// A step is stopped two ways: the operator interrupts, and the step's own
// timeout expires — `deploy.command_timeout` is the one that ends a hung
// production deploy. Both go through the same cancellation, so both have to
// leave the target as quiet as the other; this is the timeout half, and it is
// the half that runs without anyone watching.
func TestATimedOutLocalCommandTakesItsChildrenWithIt(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "work.pid")

	command := "sh -c 'trap \"\" TERM; echo $$ > " + pidFile + "; exec sleep 60'"

	done := make(chan error, 1)
	go func() {
		_, err := deploy.LocalRunner{}.Run(context.Background(), command, deploy.RunOptions{Timeout: time.Second})
		done <- err
	}()

	pid := waitForRecordedPID(t, pidFile)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a command that ran out of time must fail")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("the failure must say the command timed out, got %q", err.Error())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the command that ran out of time never returned")
	}

	waitForWorkToStop(t, pid, "timed-out command's work")
}

// waitForRecordedPID waits until the work process has written its own pid. It
// waits for the file rather than sleeping a fixed amount, so a slow shell cannot
// turn the assertion into a race.
func waitForRecordedPID(t *testing.T, path string) int {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the command never recorded the pid of its work, so the interrupt cannot be asserted")
	return 0
}

// waitForWorkToStop proves the process is gone by identity, not by pid alone:
// a recycled pid would otherwise read as "still running" and, worse, a
// best-effort cleanup could kill an unrelated process that reused it. detail
// names where the work was supposed to be stopped, so the failure says which of
// the two teardowns failed.
func waitForWorkToStop(t *testing.T, pid int, detail string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !workIsRunning(t, pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	if workIsRunning(t, pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	t.Fatalf("the interrupted %s (pid %d, `sleep`) is still running", detail, pid)
}

// workIsRunning reports whether pid is still the `sleep` the step started.
//
// The identity is the executable rather than the command line, because the step
// runs under a shell — and, over ssh, under govard's own wrapper — whose command
// line contains the text of the step: a substring lookup matches the shell and
// then watches the wrong process.
//
// A `ps` that cannot run is an unknown answer, never "the work is gone". Reading
// it as gone is how an assertion quietly stops asserting on a host without
// `ps`, so the kernel is asked instead.
func workIsRunning(t *testing.T, pid int) bool {
	t.Helper()

	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err == nil {
		return strings.TrimSpace(string(out)) == "sleep"
	}
	if killErr := syscall.Kill(pid, 0); killErr == nil || errors.Is(killErr, syscall.EPERM) {
		return true
	}
	return false
}
