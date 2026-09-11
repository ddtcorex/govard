package cmd

import (
	"sort"
	"strings"
	"testing"

	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

// TestEveryRunnableCommandDeclaresRequirements is the contract guard: every
// runnable command must state its runtime requirements, so a command can never
// ship without declaring whether it needs Docker.
func TestEveryRunnableCommandDeclaresRequirements(t *testing.T) {
	missing := []string{}
	walkCommandTree(rootCmd, func(cmd *cobra.Command) {
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		if runtime.AlwaysRunnable(cmd) || runtime.HasDeclaredRequirement(cmd) {
			return
		}
		missing = append(missing, cmd.CommandPath())
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("commands without a %s annotation (add one, do not extend the allowlist):\n  %s",
			runtime.AnnotationRequires, strings.Join(missing, "\n  "))
	}
}
