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
