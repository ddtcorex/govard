package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

// TestGatewaySubcommandsDeclareRequirements is a narrow, fast check that
// catches a missing annotation before the repo-wide
// TestEveryRunnableCommandDeclaresRequirements (which walks the whole tree)
// would; both must pass.
func TestGatewaySubcommandsDeclareRequirements(t *testing.T) {
	cases := map[string]string{
		"status":     "docker",
		"allow-key":  "none",
		"revoke-key": "none",
	}
	for _, sub := range gatewayCmd.Commands() {
		want, ok := cases[sub.Name()]
		if !ok {
			t.Fatalf("unexpected gateway subcommand %q", sub.Name())
		}
		got := sub.Annotations[runtime.AnnotationRequires]
		if got == "" {
			got = gatewayCmd.Annotations[runtime.AnnotationRequires]
		}
		if got != want {
			t.Errorf("gateway %s requires %q, want %q", sub.Name(), got, want)
		}
		delete(cases, sub.Name())
	}
	if len(cases) > 0 {
		t.Errorf("missing gateway subcommands: %v", cases)
	}
}

// TestGatewayStatusRejectsArgs pins that `gateway status` takes no
// positional arguments: a stray trailing arg is a usage error, not
// something the status report silently ignores.
func TestGatewayStatusRejectsArgs(t *testing.T) {
	var status *cobra.Command
	for _, sub := range gatewayCmd.Commands() {
		if sub.Name() == "status" {
			status = sub
		}
	}
	if status == nil {
		t.Fatal("gateway has no status subcommand")
	}
	if err := status.ValidateArgs([]string{"bogus"}); err == nil {
		t.Fatal("status with a trailing arg: got nil, want a usage error")
	}
	if err := status.ValidateArgs(nil); err != nil {
		t.Fatalf("status with no args: got %v, want nil", err)
	}
}
