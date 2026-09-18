package cmd

import (
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
			pterm.Success.Println("govard-proxy-sshd is running on 127.0.0.1:2222")
		} else {
			pterm.Warning.Println("govard-proxy-sshd is not running -- run `govard svc up` to start it")
		}
		fmt.Printf("targets:   %d\n", len(reg.Targets))
		fmt.Printf("allowlist: %d key(s)\n", len(reg.Allowlist))
		return nil
	},
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
