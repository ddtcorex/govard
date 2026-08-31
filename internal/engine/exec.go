package engine

import (
	"govard/internal/conventions"
	"os"
	"os/exec"
	"syscall"
)

// Handoff replaces the current process with the specified binary and arguments.
// This is typically used for interactive shells where we want the terminal
// state and signals to be managed directly by the child process.
func Handoff(binaryPath string, args []string) error {
	// args[0] should be the binary name as seen by the new process
	return syscall.Exec(binaryPath, args, os.Environ())
}

// ShellQuote is a wrapper around conventions.ShellQuote.
// Deprecated: new code should use conventions.ShellQuote directly; this
// wrapper is kept for backward compatibility within engine/cmd callers.
func ShellQuote(raw string) string {
	return conventions.ShellQuote(raw)
}

// ResolveDockerExecutor returns the provided executor or a default that
// shells out via exec.Command. Used by framework base_url managers to
// deduplicate the nil-check + CombinedOutput fallback.
func ResolveDockerExecutor(executor func(name string, args ...string) ([]byte, error)) func(name string, args ...string) ([]byte, error) {
	if executor != nil {
		return executor
	}
	return func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
}
