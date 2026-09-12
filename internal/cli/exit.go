// Package cli owns process exit codes so an error's shape and the process
// status can never diverge.
package cli

import "errors"

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
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return CodeError
}
