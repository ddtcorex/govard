package desktop

import (
	"fmt"
	"strings"
	"time"
)

// Timeout policy for the govard CLI subprocesses the desktop app spawns.
// Every environment action below resolves its own ceiling from the subcommand
// so a cold `govard up` (image pulls, first build) is never killed by the
// short default that `env stop` and friends still use.

const (
	govardCommandDefaultTimeout = 2 * time.Minute
	govardCommandServiceTimeout = 10 * time.Minute
	govardCommandLongTimeout    = 15 * time.Minute
)

// govardCommandTimeoutPartialStateAt is the smallest timeout whose
// operations can leave a half-started environment behind when killed.
const govardCommandTimeoutPartialStateAt = 10 * time.Minute

// govardCommandTimeout maps CLI args to the ceiling the desktop runner
// enforces. It is pure so tests pin the table without spawning processes.
func govardCommandTimeout(args []string) time.Duration {
	if len(args) == 0 {
		return govardCommandDefaultTimeout
	}
	switch strings.TrimSpace(args[0]) {
	case "up", "bootstrap":
		return govardCommandLongTimeout
	case "env":
		if len(args) > 1 {
			switch strings.TrimSpace(args[1]) {
			case "restart", "pull":
				return govardCommandLongTimeout
			}
		}
		return govardCommandDefaultTimeout
	case "svc":
		if len(args) > 1 {
			switch strings.TrimSpace(args[1]) {
			case "restart", "pull":
				return govardCommandLongTimeout
			case "up":
				return govardCommandServiceTimeout
			}
		}
		return govardCommandDefaultTimeout
	default:
		return govardCommandDefaultTimeout
	}
}

// newGovardCommandTimeoutError builds the error the runner returns when the
// context deadline fires. Long operations get an explicit partial-state hint
// because killing `up` mid-flight leaves containers behind; short ones keep
// the plain message.
func newGovardCommandTimeoutError(timeout time.Duration, trimmedOutput string) error {
	msg := fmt.Sprintf("command timed out after %v: %s", timeout, trimmedOutput)
	if timeout >= govardCommandTimeoutPartialStateAt {
		msg += " The operation was stopped mid-flight; the environment may be partially started. Check the dashboard for orphaned resources."
	}
	return fmt.Errorf("%s", msg)
}
