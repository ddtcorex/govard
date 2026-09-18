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

// TestFormatPortWarning pins the exact port-health wording: a published
// binding is quiet, a missing one names the container, the port, and the
// likely cause (another holder on 127.0.0.1:2222).
func TestFormatPortWarning(t *testing.T) {
	if got := formatPortWarning(true); got != "" {
		t.Fatalf("published binding: got %q, want quiet", got)
	}
	want := "WARNING: govard-proxy-sshd is running but port 2222 is not published -- another process is likely holding 127.0.0.1:2222"
	if got := formatPortWarning(false); got != want {
		t.Fatalf("missing binding: got %q, want %q", got, want)
	}
}

// TestGatewayStatusPortWarningBranch pins the status output branch with a
// stubbed port probe (no Docker daemon): only running-without-binding warns.
func TestGatewayStatusPortWarningBranch(t *testing.T) {
	want := "WARNING: govard-proxy-sshd is running but port 2222 is not published -- another process is likely holding 127.0.0.1:2222"
	cases := []struct {
		name      string
		running   bool
		published bool
		want      string
	}{
		{"running without binding warns", true, false, want},
		{"running with binding stays quiet", true, true, ""},
		{"stopped stays quiet", false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gatewayStatusPortWarning(tc.running, func() bool { return tc.published })
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
