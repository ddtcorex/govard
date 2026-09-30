//go:build unix

package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

// Killing the local `docker exec` client stops nothing inside the container: the
// step belongs to the daemon, so composer kept writing into --output after a
// timeout or Ctrl-C. The fix mirrors SSHRunner: the step records its own pid,
// and the cancel path runs a second, short-lived `docker exec` that signals the
// process group that pid leads.
//
// The stand-in for docker reproduces the one property the fix depends on, and
// the one measured on a real daemon: a `docker exec` process is its own session
// and process group leader (pid == pgid == sid inside the container), which is
// what `setsid -w` gives the command here. Killing the stand-in (the client)
// therefore leaves the step running, exactly as the daemon does.

// TestContainerCancelKillsInnerProcess drives both ways a step is stopped, and
// asserts the teardown exec was issued, that it read the very record the step
// wrote, and that the work that record names is actually gone.
func TestContainerCancelKillsInnerProcess(t *testing.T) {
	cases := []struct {
		name     string
		timeout  time.Duration
		cancel   bool
		sentence string
	}{
		{name: "an interrupted run", cancel: true, sentence: "interrupted"},
		{name: "a step that timed out", timeout: 700 * time.Millisecond, sentence: "timed out"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			logFile, dir := containerStandIn(t)
			workPID := filepath.Join(dir, "work.pid")
			command := "sleep 60 & echo $! > " + workPID + "; wait"
			records := containerInterruptRecords()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			start := time.Now()
			done := make(chan error, 1)
			go func() {
				_, runErr := containerRunnerFixture().Run(ctx, command, deploy.RunOptions{Timeout: testCase.timeout})
				done <- runErr
			}()

			pid := waitForRecordedPID(t, workPID)
			assertRecordNamesTheWork(t, pid)
			if testCase.cancel {
				cancel()
			}

			var runErr error
			select {
			case runErr = <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("the stopped step never returned")
			}
			if elapsed := time.Since(start); elapsed > 6*time.Second {
				t.Fatalf("the stopped step took %s to return; the teardown must stay bounded", elapsed)
			}

			var commandErr *deploy.CommandError
			if !errors.As(runErr, &commandErr) {
				t.Fatalf("err = %v, want *deploy.CommandError", runErr)
			}
			message := runErr.Error()
			if !strings.Contains(message, testCase.sentence) {
				t.Fatalf("the failure must say %q, got %q", testCase.sentence, message)
			}
			// The operator is told the teardown was attempted, and where.
			for _, want := range []string{"teardown attempted: ", "container sample-php-1"} {
				if !strings.Contains(message, want) {
					t.Fatalf("the failure must say the teardown ran (%q), got %q", want, message)
				}
			}
			if strings.Contains(message, "govard-build-interrupt") {
				t.Fatalf("the operator must see the step they wrote, not govard's wrapper, got %q", message)
			}

			// The work the step started is gone: the teardown reached the group
			// inside the "container", which killing the client cannot do.
			waitForWorkToStop(t, pid, "container step's work")

			calls := readContainerStandInLog(t, logFile)
			step, teardown := splitContainerCalls(t, calls)
			recorded := containerPIDRecord.FindStringSubmatch(step)
			if recorded == nil {
				t.Fatalf("the step was not wrapped with a process-group record:\n%s", step)
			}
			// Not just any exec: the same container, as the same account, reading
			// the record this step wrote and signalling the group it names.
			for _, want := range []string{
				"exec -u www-data ",
				" sample-php-1 sh -c ",
				"cat '" + recorded[1] + "'",
				"kill -TERM -$p",
			} {
				if !strings.Contains(teardown, want) {
					t.Fatalf("the teardown exec must contain %q, got:\n%s", want, teardown)
				}
			}
			waitForContainerRecordsToSettle(t, records)
		})
	}
}

// containerPIDRecord captures the record path the step wrapper writes `$$` into.
var containerPIDRecord = regexp.MustCompile(`printf '%s\\n' \$\$ > '(/tmp/\.govard-build-interrupt-[0-9]+-[0-9]+\.pid)'`)

// containerStandIn puts a fake `docker` first on PATH that logs every call on
// one line and runs an exec's `sh -c` script on this machine in a session of its
// own, the way the daemon runs it inside the container. The output is relayed
// through `cat`, which stays in the client's group the way the real client's
// relay does: killing the client closes govard's pipes, and the step in its own
// session does not hold them open.
func containerStandIn(t *testing.T) (string, string) {
	t.Helper()

	setsid, err := exec.LookPath("setsid")
	if err != nil {
		t.Skip("setsid stands in for the daemon's session leader and is not available on this host")
	}

	dir := t.TempDir()
	logFile := filepath.Join(dir, "docker.log")
	shimDir := filepath.Join(dir, "bin")
	writeFile(t, filepath.Join(shimDir, "docker"), `#!/bin/sh
printf '%s\n' "$*" | tr '\n' ' ' >> "$GOVARD_FAKE_DOCKER_LOG"
printf '\n' >> "$GOVARD_FAKE_DOCKER_LOG"
for last do :; done
`+setsid+` -w sh -c "$last" 2>&1 | cat
`)
	if err := os.Chmod(filepath.Join(shimDir, "docker"), 0o755); err != nil {
		t.Fatalf("make the docker stand-in executable: %v", err)
	}
	t.Setenv("GOVARD_FAKE_DOCKER_LOG", logFile)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile, dir
}

func readContainerStandInLog(t *testing.T, logFile string) []string {
	t.Helper()

	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read the docker log: %v", err)
	}
	var calls []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

// splitContainerCalls separates the step's exec from the teardown's. Exactly one
// of each is expected: a second step exec would mean the runner retried, and a
// missing teardown is the defect this file exists for.
func splitContainerCalls(t *testing.T, calls []string) (string, string) {
	t.Helper()

	var steps, teardowns []string
	for _, call := range calls {
		if strings.Contains(call, "kill -TERM") {
			teardowns = append(teardowns, call)
			continue
		}
		steps = append(steps, call)
	}
	if len(steps) != 1 || len(teardowns) != 1 {
		t.Fatalf("want one step exec and one teardown exec, got %d and %d:\n%s",
			len(steps), len(teardowns), strings.Join(calls, "\n"))
	}
	return steps[0], teardowns[0]
}

func containerInterruptRecords() map[string]bool {
	matches, err := filepath.Glob("/tmp/.govard-build-interrupt-*.pid")
	if err != nil {
		return map[string]bool{}
	}
	found := make(map[string]bool, len(matches))
	for _, match := range matches {
		found[match] = true
	}
	return found
}

// waitForContainerRecordsToSettle holds the teardown to removing the record it
// read, so a stopped build leaves nothing behind in the container's /tmp.
func waitForContainerRecordsToSettle(t *testing.T, before map[string]bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		left := false
		for path := range containerInterruptRecords() {
			if !before[path] {
				left = true
			}
		}
		if !left {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for path := range containerInterruptRecords() {
		if !before[path] {
			os.Remove(path)
			t.Fatalf("the teardown left its record behind: %s", path)
		}
	}
}
