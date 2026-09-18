package cmd

import (
	"context"
	"fmt"

	"govard/internal/engine"
	"govard/internal/gateway"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var gatewayCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "gateway",
	Short: "Manage the shared SSH gateway (govard-proxy-sshd)",
	Long: `The shared SSH gateway is a global bastion that lets "ssh <project>@ssh.govard.test -p 2222"
reach any running sandbox, without ever chasing an ephemeral per-container
port. It rides the same global proxy stack as Caddy: start it with
"govard svc up".`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the SSH gateway container is running",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := gateway.Load()
		if err != nil {
			return err
		}
		if engine.IsContainerRunning(cmd.Context(), "govard-proxy-sshd") {
			if w := gatewayStatusPortWarning(true, func() bool {
				return gatewaySSHPortPublished(cmd.Context())
			}); w != "" {
				pterm.Warning.Println(w)
			} else {
				pterm.Success.Println("govard-proxy-sshd is running on 127.0.0.1:2222")
			}
		} else {
			pterm.Warning.Println("govard-proxy-sshd is not running -- run `govard svc up` to start it")
		}
		fmt.Printf("targets:   %d\n", len(reg.Targets))
		fmt.Printf("allowlist: %d key(s)\n", len(reg.Allowlist))
		return nil
	},
}

// gatewaySSHPortPublished is the `gateway status` probe for the sshd
// service's 127.0.0.1:2222 binding (shell-free, via the vendored docker
// client in internal/engine). A variable so tests can stub the daemon away.
var gatewaySSHPortPublished = func(ctx context.Context) bool {
	return engine.IsGatewaySSHPortPublished(ctx)
}

// formatPortWarning shapes the loud port-health warning `gateway status`
// prints when govard-proxy-sshd runs without its 127.0.0.1:2222 binding
// (usually because another process holds the port and compose started the
// service without it). Pure so the wording stays pinned hermetically.
func formatPortWarning(published bool) string {
	if published {
		return ""
	}
	return "WARNING: govard-proxy-sshd is running but port 2222 is not published -- another process is likely holding 127.0.0.1:2222"
}

// gatewayStatusPortWarning decides the port-health line for a status report:
// when the container runs but its 2222 binding is missing (probe reports
// false), it returns the loud warning; otherwise "". The probe is a stub
// point so tests pin the branch without a Docker daemon.
func gatewayStatusPortWarning(running bool, probe func() bool) string {
	if !running {
		return ""
	}
	return formatPortWarning(probe())
}

var gatewayAllowKeyCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "allow-key <public-key-line>",
	Short: "Add a client public key to the gateway allowlist",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := gateway.Load()
		if err != nil {
			return err
		}
		fingerprint, err := reg.AllowKey(args[0])
		if err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return err
		}
		pterm.Success.Printf("allowed %s\n", fingerprint)
		return nil
	},
}

var gatewayRevokeKeyCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "revoke-key <fingerprint-or-comment>",
	Short: "Remove client public keys matching a fingerprint or comment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := gateway.Load()
		if err != nil {
			return err
		}
		if err := reg.RevokeKey(args[0]); err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return err
		}
		pterm.Success.Printf("revoked %s\n", args[0])
		return nil
	},
}

func init() {
	gatewayCmd.AddCommand(gatewayStatusCmd)
	gatewayCmd.AddCommand(gatewayAllowKeyCmd)
	gatewayCmd.AddCommand(gatewayRevokeKeyCmd)
}
