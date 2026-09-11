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
