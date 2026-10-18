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
	case "db":
		// Desktop database transfers stream whole dumps through docker exec;
		// like `up` they must never be killed by the short default.
		return govardCommandLongTimeout
	case "init":
		// Scaffolding plus migration scans over large checkouts outgrow the
		// short default; the work is rerunnable, not instant.
		return govardCommandServiceTimeout
	case "debug":
		// Toggling Xdebug recreates the PHP container, the same class of
		// work as `svc up`, so it shares that ceiling.
		return govardCommandServiceTimeout
	case "snapshot":
		// Snapshot create/restore/export move the same whole dumps (plus a
		// media tree) through docker exec and tar, so they share the
		// transfer ceiling rather than the short default.
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

// commandTimeoutOutputMax caps CLI output quoted in a timeout error: the
// text reaches toasts, so it is sanitized like every other UI-bound error
// and bounded even when the CLI dumps pages before dying.
const commandTimeoutOutputMax = 2000

// newGovardCommandTimeoutError builds the error the runner returns when the
// context deadline fires. Long operations get an explicit partial-state hint
// because killing `up` mid-flight leaves containers behind; short ones keep
// the plain message.
func newGovardCommandTimeoutError(timeout time.Duration, trimmedOutput string) error {
	msg := fmt.Sprintf("command timed out after %v: %s", timeout, sanitizeUIText(trimmedOutput, commandTimeoutOutputMax))
	if timeout >= govardCommandTimeoutPartialStateAt {
		msg += " The operation was stopped mid-flight; the environment may be partially started. Check the dashboard for orphaned resources."
	}
	return fmt.Errorf("%s", msg)
}
