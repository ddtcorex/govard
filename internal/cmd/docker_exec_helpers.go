package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func dockerExecBaseArgs() []string {
	// If we are running in an integration test, always use non-interactive mode
	if os.Getenv("GOVARD_TEST_RUNTIME") == "true" {
		return []string{"exec", "-i"}
	}

	// Check if we have a TTY for stdin/stdout
	if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
		return []string{"exec", "-it"}
	}
	return []string{"exec", "-i"}
}

// dockerExecArgs builds the `docker exec` flag prefix for CLI wrappers.
//
// Interactive sessions (a bare `redis cli`, `varnish log`/`stats`) keep the
// historical -i/-it behavior with stdin attached, so piped input such as
// `echo PING | govard redis cli` keeps working.
//
// One-shot commands (PING, HGET, --scan, ban, ...) run detached: no -i/-t and
// no stdin. Attaching stdin lets `docker exec -i` drain a pipe that feeds the
// caller's own `while read` loop, silently dropping loop input (e.g. `govard
// redis --scan | while read ... govard redis HGET ...` returns nothing while
// the same loop over a direct client returns full results). Dropping -t also
// keeps the inner CLI on raw output formatting instead of human-readable TTY
// formatting, matching a direct client call.
func dockerExecArgs(interactive bool) []string {
	if !interactive {
		return []string{"exec"}
	}
	return dockerExecBaseArgs()
}

// attachExecStdin wires stdio for a `docker exec` child: stdout/stderr always,
// stdin only for interactive sessions (see dockerExecArgs).
func attachExecStdin(c *exec.Cmd, interactive bool) {
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if interactive {
		c.Stdin = os.Stdin
	}
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func ensureContainerReadyForExec(containerName string, serviceLabel string) error {
	inspect := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	output, err := inspect.CombinedOutput()
	state := strings.ToLower(strings.TrimSpace(string(output)))

	if state == "true" {
		return nil
	}
	if state == "false" {
		return fmt.Errorf(
			"%s container %s is stopped. Run `govard env up` (or `govard env restart`) and retry",
			serviceLabel,
			containerName,
		)
	}

	if err != nil {
		return fmt.Errorf(
			"%s container %s is unknown. Run `govard env up` (or `govard env restart`) and retry",
			serviceLabel,
			containerName,
		)
	}

	return fmt.Errorf(
		"%s container %s is %s. Run `govard env up` (or `govard env restart`) and retry",
		serviceLabel,
		containerName,
		state,
	)
}
