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
// remote process group and signals it over a second connection, and this drives
// that path end to end against a stand-in for sshd.
//
// The stand-in reproduces the one property the fix depends on, and the one that
// was measured on a real sshd: the command runs as its own session and process
// group leader (`setsid`), which is what makes the pid the wrapper records a
// group id. Without it the kill would be aimed at a group govard did not create
// — which is why the fix is written to signal `-pid` and never `pid` alone.
func TestAnInterruptedRemoteCommandTakesItsProcessGroupWithIt(t *testing.T) {
	setsid, err := exec.LookPath("setsid")
	if err != nil {
		t.Skip("setsid stands in for sshd's session leader and is not available on this host")
	}

	dir := t.TempDir()
	logFile := filepath.Join(dir, "ssh.log")
	shimDir := filepath.Join(dir, "bin")
	shim := filepath.Join(shimDir, "ssh")

	// The stand-in sshd: log the invocation, then run the command as the last ssh
	// argument would run on the other side — in a session of its own.
	writeFile(t, shim, `#!/bin/sh
printf '%s\n' "$*" >> "$GOVARD_FAKE_SSH_LOG"
for last do :; done
exec `+setsid+` -w sh -c "$last"
`)
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatalf("make the ssh stand-in executable: %v", err)
	}
	t.Setenv("GOVARD_FAKE_SSH_LOG", logFile)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	workPIDFile := filepath.Join(dir, "work.pid")
	command := "echo $$ > " + workPIDFile + "; exec sleep 300"

	runner := deploy.SSHRunner{
		RemoteName: "sandbox",
		Config: engine.RemoteConfig{
			Host: "sandbox.example.com",
			User: "deploy",
			Port: 2222,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(ctx, command, deploy.RunOptions{})
		done <- runErr
	}()

	pid := waitForRecordedPID(t, workPIDFile)
	cancel()

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Fatal("an interrupted command must fail")
		}
		if !strings.Contains(runErr.Error(), "interrupted") {
			t.Fatalf("the failure must say the run was interrupted, got %q", runErr.Error())
		}
		if strings.Contains(runErr.Error(), "govard-deploy-interrupt") {
			t.Fatalf("the operator must see the step they wrote, not govard's wrapper, got %q", runErr.Error())
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the interrupted command never returned")
	}

	waitForWorkToStop(t, pid, "remote command's work")

	// The teardown is a second connection, not a signal from here: it is the only
	// thing that can reach work on the other machine. Asserted on what was sent,
	// because that is the part a real server would have obeyed.
	raw, readErr := os.ReadFile(logFile)
	if readErr != nil {
		t.Fatalf("read the ssh log: %v", readErr)
	}
	sent := string(raw)
	if !strings.Contains(sent, "kill -TERM -$p") {
		t.Fatalf("the interrupted run never told the remote host to signal the process group:\n%s", sent)
	}
	if !strings.Contains(sent, "trap 'rm -f ") {
		t.Fatalf("the remote command was not wrapped with a process-group record:\n%s", sent)
	}
}
