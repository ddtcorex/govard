package cmd

import (
	"testing"

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
