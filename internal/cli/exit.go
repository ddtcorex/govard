// Package cli owns process exit codes so an error's shape and the process
// status can never diverge.
package cli

import (
	"os/exec"
)

const (
	CodeOK         = 0
	CodeError      = 1
	CodeUsage      = 2
	CodeCapability = 3
	CodeConfig     = 4
)

// UsageError marks a flag or argument error.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }
func (e *UsageError) ExitCode() int { return CodeUsage }

// ConfigError marks a project configuration error: a missing or unreadable
// `.govard.yml`, a value the configuration layer cannot parse, or a hook the
// plan builder refuses.
//
// It is separate from UsageError because the remedy differs — the operator edits
// the file rather than the command line — and because CI has to tell the two
// apart without reading prose. `config_error` is the only error class that uses
// exit code 4.
type ConfigError struct{ Err error }

func (e *ConfigError) Error() string { return e.Err.Error() }
func (e *ConfigError) Unwrap() error { return e.Err }
func (e *ConfigError) ExitCode() int { return CodeConfig }

// Code maps an error to its process exit code. Errors carrying an ExitCode
// method win; anything else is a plain execution error.
func Code(err error) int {
	if err == nil {
		return CodeOK
	}
	if coded := govardCoded(err); coded != nil {
		return coded.ExitCode()
	}
	return CodeError
}

// govardCoded finds the first error in the chain that carries a govard exit code.
//
// A *exec.ExitError also has an ExitCode method, but it reports what a child
// process chose to exit with (255 from ssh, 22 from curl, 127 for a missing
// binary), which is not part of the documented 0 to 4 set. It is skipped, so a
// failed remote step exits 1 and keeps the child's status in the message.
func govardCoded(err error) interface{ ExitCode() int } {
	if err == nil {
		return nil
	}
	if _, isChild := err.(*exec.ExitError); !isChild {
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			return coded
		}
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		return govardCoded(wrapped.Unwrap())
	case interface{ Unwrap() []error }:
		for _, inner := range wrapped.Unwrap() {
			if coded := govardCoded(inner); coded != nil {
				return coded
			}
		}
	}
	return nil
}
