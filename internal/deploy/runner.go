package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// waitDelayAfterKill bounds how long Wait keeps waiting for inherited pipes
// after the command was killed or its context expired.
const waitDelayAfterKill = time.Second

// Runner executes one shell command on one host. Every deploy step goes through
// this interface, which is what makes the whole pipeline testable without a
// server: the hermetic end-to-end tests use LocalRunner, production uses
// SSHRunner, and the plan does not care which.
type Runner interface {
	Run(ctx context.Context, command string, opts RunOptions) (Result, error)
}

// Result is the outcome of one command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunOptions controls one command invocation.
type RunOptions struct {
	// Dir is the working directory. Empty means the host's default.
	Dir string
	// Timeout bounds the command. Zero means the caller's context alone bounds
	// it; the executor always sets one, because an unbounded remote command is
	// how a deploy hangs forever.
	Timeout time.Duration
	// Stdin is fed to the command's standard input. It exists for one thing:
	// handing a secret to a remote command without putting it in argv (visible
	// in the target's process list) or in the command text (printed by
	// --verbose and kept in CI logs).
	Stdin string
}

// CommandError reports a command that ran and failed. It always carries the
// command text and the exit code: "no silent failures" means the operator must
// be able to see exactly what failed, on which host, with what output.
type CommandError struct {
	Command  string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	// A timeout is a different remedy from a failing command, so it must be
	// visible in the message and not only in the wrapped error.
	prefix := fmt.Sprintf("command failed (exit %d)", e.ExitCode)
	if errors.Is(e.Err, context.DeadlineExceeded) || errors.Is(e.Err, exec.ErrWaitDelay) {
		prefix = "command timed out"
	}
	message := prefix + ": " + e.Command
	if trimmed := strings.TrimSpace(e.Stderr); trimmed != "" {
		message += "\n" + trimmed
	}
	return message
}

func (e *CommandError) Unwrap() error { return e.Err }

// LocalRunner runs commands through the local shell.
type LocalRunner struct{}

// Run implements Runner for the local machine.
func (LocalRunner) Run(ctx context.Context, command string, opts RunOptions) (Result, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	// A killed process does not necessarily close the pipes its children
	// inherited, and Wait would then block until those children exit — a
	// 500ms timeout observed as 30s. WaitDelay bounds that wait; this is the
	// same failure the desktop doctor probe hit.
	cmd.WaitDelay = waitDelayAfterKill

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	return result, commandError(ctx, command, result.Stderr, err)
}

// commandError converts an exec error into a *CommandError, keeping the exit
// code and naming a timeout explicitly: a killed command and a failed command
// need different remedies. A context deadline surfaces as "signal: killed", so
// the context is what tells the two apart.
func commandError(ctx context.Context, command, stderr string, err error) error {
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &CommandError{Command: command, ExitCode: exitCode, Stderr: stderr, Err: fmt.Errorf("timed out: %w", context.DeadlineExceeded)}
	}
	return &CommandError{Command: command, ExitCode: exitCode, Stderr: stderr, Err: err}
}
