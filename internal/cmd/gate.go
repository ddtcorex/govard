package cmd

import (
	"errors"
	"fmt"

	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// gateError enforces a command's declared runtime requirements before its
// workflow starts. It returns nil when the command may run.
func gateError(cmd *cobra.Command) error {
	if cmd == nil || runtime.AlwaysRunnable(cmd) {
		return nil
	}
	capabilities := runtime.Requires(cmd)
	if len(capabilities) == 0 {
		return nil
	}
	err := runtime.Probe(capabilities...)
	if err == nil {
		return nil
	}
	var missing *runtime.MissingError
	// In machine mode the hint travels inside the JSON envelope, so stdout
	// stays parseable.
	if errors.As(err, &missing) && missing.Hint != "" && !errorJSON {
		pterm.Info.Printf("Hint: %s\n", missing.Hint)
	}
	return fmt.Errorf("cannot run %q: %w", cmd.CommandPath(), err)
}

// requireDocker is the flag-driven half of the capability contract: a command
// that needs a container for one of its flags, and for no other reason, asks
// for it here. The answer is the same *runtime.MissingError the root gate
// produces, so the hint, the exit code and the machine-readable envelope are
// the ones an operator already knows.
//
// `govard audit run --checks` is the first command that needed this shape, and
// `govard deploy build --runner container` reuses it rather than inventing a
// second mechanism. Everywhere else the fix is an annotation, never a probe.
func requireDocker(hint string) error {
	err := runtime.Probe(runtime.CapDocker)
	if err == nil {
		return nil
	}
	var missing *runtime.MissingError
	detail := err.Error()
	if errors.As(err, &missing) {
		detail = missing.Detail
	}
	// In machine mode the hint travels inside the JSON envelope, so stdout
	// stays parseable.
	if !errorJSON {
		pterm.Info.Printf("Hint: %s\n", hint)
	}
	return &runtime.MissingError{
		Caps:   []runtime.Capability{runtime.CapDocker},
		Detail: detail,
		Hint:   hint,
	}
}
