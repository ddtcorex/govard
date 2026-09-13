package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func TestLocalRunnerCapturesOutputAndWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	result, err := deploy.LocalRunner{}.Run(context.Background(), "cat marker.txt", deploy.RunOptions{Dir: dir})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "here" {
		t.Fatalf("stdout = %q, want here", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestLocalRunnerReportsNonZeroExitAsCommandError(t *testing.T) {
	_, err := deploy.LocalRunner{}.Run(context.Background(), "exit 7", deploy.RunOptions{Dir: t.TempDir()})
	var cmdErr *deploy.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v, want *deploy.CommandError", err)
	}
	if cmdErr.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", cmdErr.ExitCode)
	}
	if !strings.Contains(cmdErr.Error(), "exit 7") {
		t.Fatalf("error must name the failing command, got %q", cmdErr.Error())
	}
}

func TestLocalRunnerHonoursRunOptionsTimeout(t *testing.T) {
	start := time.Now()
	_, err := deploy.LocalRunner{}.Run(context.Background(), "sleep 30", deploy.RunOptions{Dir: t.TempDir(), Timeout: 500 * time.Millisecond})
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s; the runner must kill the process, not wait for it", elapsed)
	}
	var cmdErr *deploy.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v, want *deploy.CommandError", err)
	}
	if !strings.Contains(cmdErr.Error(), "timed out") {
		t.Fatalf("error must name the timeout, got %q", cmdErr.Error())
	}
}

// A caller detects an interrupted or timed-out run with `errors.Is`, which only
// works because the command error unwraps to the error underneath it. That is the
// documented way to tell "the run was interrupted" from "the command failed", so
// the wrapper has to stay see-through.
func TestCommandErrorUnwrapsToWhatCausedIt(t *testing.T) {
	interrupted := &deploy.CommandError{Command: "sleep 60", ExitCode: -1, Err: context.Canceled}
	if !errors.Is(interrupted, context.Canceled) {
		t.Fatal("an interrupted command must be detectable with errors.Is(err, context.Canceled)")
	}
	if !strings.Contains(interrupted.Error(), "interrupted") {
		t.Fatalf("message = %q, want it to say the run was interrupted", interrupted.Error())
	}
	timedOut := &deploy.CommandError{Command: "composer install", ExitCode: -1, Err: context.DeadlineExceeded}
	if !errors.Is(timedOut, context.DeadlineExceeded) {
		t.Fatal("a timed-out command must be detectable with errors.Is(err, context.DeadlineExceeded)")
	}
	if !strings.Contains(timedOut.Error(), "timed out") {
		t.Fatalf("message = %q, want it to say the command timed out", timedOut.Error())
	}
}
