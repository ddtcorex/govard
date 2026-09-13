//go:build unix

package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
)

// A deploy over SSH runs its work on the other machine, so killing the local ssh
// client stops nothing: measured on a real target, the remote shell and its child
// were both still running after the client was killed. The fix records the
// remote process group and signals it over a second connection, and these drive
// that path end to end against a stand-in for sshd.
//
// The stand-in reproduces the one property the fix depends on, and the one that
// was measured on a real sshd: the command runs as its own session and process
// group leader (`setsid`), which is what makes the pid the wrapper records a
// group id. Without it the kill would be aimed at a group govard did not create
// — which is why the fix is written to signal `-pid` and never `pid` alone.

// TestAnInterruptedRemoteCommandTakesItsProcessGroupWithIt is the case the fix
// exists for: the run is cancelled while the work is in flight, and the work is
// on the other side of a connection that is about to die.
func TestAnInterruptedRemoteCommandTakesItsProcessGroupWithIt(t *testing.T) {
	runner, _, dir := sandboxedSSHRunner(t, "")
	command := remoteWorkCommand(t, dir)
	records := interruptRecords()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(ctx, command, deploy.RunOptions{})
		done <- runErr
	}()

	pid := waitForRecordedPID(t, filepath.Join(dir, "work.pid"))
	assertRecordNamesTheWork(t, pid)
	cancel()

	waitForInterruptedRun(t, done)
	waitForWorkToStop(t, pid, "remote command's work")
	waitForRecordsToSettle(t, records)

	// The teardown is a second connection, not a signal from here: it is the only
	// thing that can reach work on the other machine. Asserted on what was sent,
	// because that is the part a real server would have obeyed.
	sent := readSSHLog(t)
	if !strings.Contains(sent, "kill -TERM -$p") {
		t.Fatalf("the interrupted run never told the remote host to signal the process group:\n%s", sent)
	}
	if !strings.Contains(sent, "rm -f /tmp/.govard-deploy-interrupt-") {
		t.Fatalf("the remote command was not wrapped with a process-group record:\n%s", sent)
	}
}

// A step can end in ways that replace the shell govard wrapped, and the record it
// leaves in the target's /tmp is harmless only while it is short-lived: one file
// per deploy, forever, is litter an operator cannot explain. `exec` and a step's
// own EXIT trap are both legitimate things for a project's deploy hook to do, and
// both defeated the trap this used to rely on.
func TestARemoteStepLeavesNoRecordWhateverItsOwnShellDoes(t *testing.T) {
	runner, _, _ := sandboxedSSHRunner(t, "")

	for _, command := range []string{"true", "exec true", "trap 'true' EXIT; true"} {
		before := interruptRecords()
		if _, err := runner.Run(context.Background(), command, deploy.RunOptions{}); err != nil {
			t.Fatalf("run %q: %v", command, err)
		}
		for path := range interruptRecords() {
			if before[path] {
				continue
			}
			os.Remove(path)
			t.Fatalf("a run that succeeded left its interrupt record behind after %q: %s", command, path)
		}
	}
}

// A cancel can land in the window between the connection being established and
// the remote shell executing its first statement — a slow sshd, a loaded target.
// Giving up on the record then leaves a step running that nothing can reach, so
// the teardown waits for it instead of concluding there is nothing to stop.
func TestARemoteStepThatHasNotRecordedItsPIDYetIsStillStopped(t *testing.T) {
	runner, _, dir := sandboxedSSHRunner(t, "1.5")
	workPIDFile := filepath.Join(dir, "work.pid")
	command := remoteWorkCommand(t, dir)
	records := interruptRecords()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(ctx, command, deploy.RunOptions{})
		done <- runErr
	}()

	// The interrupt has to land before the step has produced anything, or this is
	// not the window being tested.
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(workPIDFile); err == nil {
		t.Fatal("the step had already run when the interrupt was sent, so the record was not late")
	}
	cancel()

	waitForInterruptedRun(t, done)
	pid := waitForRecordedPID(t, workPIDFile)
	waitForWorkToStop(t, pid, "late-starting remote command's work")
	waitForRecordsToSettle(t, records)
}

// sandboxedSSHRunner points SSHRunner at a stand-in for sshd. startDelay is how
// long the stand-in takes, inside the new session, before it runs the command —
// a non-empty value reproduces a remote side that is slow to get going.
func sandboxedSSHRunner(t *testing.T, startDelay string) (deploy.SSHRunner, string, string) {
	t.Helper()

	setsid, err := exec.LookPath("setsid")
	if err != nil {
		t.Skip("setsid stands in for sshd's session leader and is not available on this host")
	}

	dir := t.TempDir()
	logFile := filepath.Join(dir, "ssh.log")
	shimDir := filepath.Join(dir, "bin")
	shim := filepath.Join(shimDir, "ssh")

	// The stand-in sshd: log the invocation, then run the command the way the
	// last ssh argument runs on the other side — in a session of its own.
	// The delay is applied only to the invocation the marker names: the teardown
	// is a second connection to the same stand-in, and delaying that one too
	// would hide the very window the test is about.
	writeFile(t, shim, `#!/bin/sh
printf '%s\n' "$*" >> "$GOVARD_FAKE_SSH_LOG"
for last do :; done
delay=0
case "$last" in
  *"$GOVARD_FAKE_SSH_SLOW_MARKER"*) delay="${GOVARD_FAKE_SSH_DELAY:-0}" ;;
esac
exec `+setsid+` -w sh -c "sleep $delay; $last"
`)
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatalf("make the ssh stand-in executable: %v", err)
	}
	t.Setenv("GOVARD_FAKE_SSH_LOG", logFile)
	t.Setenv("GOVARD_FAKE_SSH_DELAY", startDelay)
	if startDelay != "" {
		// The step's own invocation is the one to delay, and the pid file path
		// appears in that command and in the teardown's script never.
		t.Setenv("GOVARD_FAKE_SSH_SLOW_MARKER", filepath.Join(dir, "work.pid"))
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return deploy.SSHRunner{
		RemoteName: "sandbox",
		Config: engine.RemoteConfig{
			Host: "sandbox.example.com",
			User: "deploy",
			Port: 2222,
		},
	}, logFile, dir
}

// waitForInterruptedRun blocks until the cancelled run returns, and holds it to
// the two things the operator sees: the sentence that says what happened, and
// their own command rather than govard's wrapper around it.
func waitForInterruptedRun(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an interrupted command must fail")
		}
		if !strings.Contains(err.Error(), "interrupted") {
			t.Fatalf("the failure must say the run was interrupted, got %q", err.Error())
		}
		if strings.Contains(err.Error(), "govard-deploy-interrupt") {
			t.Fatalf("the operator must see the step they wrote, not govard's wrapper, got %q", err.Error())
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the interrupted command never returned")
	}
}

// waitForRecordsToSettle waits for the teardown of an interrupted step to remove
// the record it read. That teardown runs on its own connection after the run has
// already returned, so a test that ends first leaves the file behind on whatever
// target it was pointed at — and the file's removal is part of the behaviour
// being asserted, not housekeeping around it.
func waitForRecordsToSettle(t *testing.T, before map[string]bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(recordsAddedSince(before)) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	left := recordsAddedSince(before)
	for path := range left {
		os.Remove(path)
	}
	t.Fatalf("the teardown left %d record(s) behind: %v", len(left), left)
}

func recordsAddedSince(before map[string]bool) map[string]bool {
	added := map[string]bool{}
	for path := range interruptRecords() {
		if !before[path] {
			added[path] = true
		}
	}
	return added
}

// remoteWorkCommand starts the work in the background and records its pid. `$$`
// would name the shell rather than the work: the step runs in a subshell, and a
// subshell keeps its parent's `$$`. The sleep is short on purpose: a test that
// fails before the pid is known cannot clean up after itself.
func remoteWorkCommand(t *testing.T, dir string) string {
	t.Helper()

	pidFile := filepath.Join(dir, "work.pid")
	return "sleep 60 & echo $! > " + pidFile + "; wait"
}

// assertRecordNamesTheWork proves the pid the record holds is the work and not
// the shell that started it, before anything is claimed about it.
func assertRecordNamesTheWork(t *testing.T, pid int) {
	t.Helper()

	if !workIsRunning(t, pid) {
		t.Fatalf("pid %d is not the sleep this test started, so the assertion would watch the wrong process", pid)
	}
}

// interruptRecords lists the records govard writes on a target. The path is
// literal rather than os.TempDir() because that is the path the runner builds.
func interruptRecords() map[string]bool {
	matches, err := filepath.Glob("/tmp/.govard-deploy-interrupt-*.pid")
	if err != nil {
		return map[string]bool{}
	}
	found := make(map[string]bool, len(matches))
	for _, match := range matches {
		found[match] = true
	}
	return found
}

func readSSHLog(t *testing.T) string {
	t.Helper()

	logFile := os.Getenv("GOVARD_FAKE_SSH_LOG")
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read the ssh log: %v", err)
	}
	return string(raw)
}
