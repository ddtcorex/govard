package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/tunnel"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type tunnelCommandDependencies struct {
	NewProvider func(ref engine.ProviderRef) (tunnel.Provider, error)
	RunCommand  func(command *exec.Cmd) error
}

var tunnelDeps = tunnelCommandDependencies{
	NewProvider: tunnel.NewProvider,
	RunCommand: func(command *exec.Cmd) error {
		return command.Run()
	},
}

// TunnelDependenciesForTest allows tests to swap tunnel command dependencies.
type TunnelDependenciesForTest struct {
	NewProvider func(ref engine.ProviderRef) (tunnel.Provider, error)
	RunCommand  func(command *exec.Cmd) error
}

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Manage local project tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var tunnelStartCmd = &cobra.Command{
	Use:   "start [url]",
	Short: "Start a public tunnel to the local project",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()
		operationStatus := engine.OperationStatusFailure
		operationMessage := ""
		operationCategory := ""

		defer func() {
			if err != nil && operationMessage == "" {
				operationMessage = err.Error()
			}
			if err == nil && operationStatus == engine.OperationStatusFailure {
				operationStatus = engine.OperationStatusSuccess
			}
			if err != nil && operationCategory == "" {
				operationCategory = classifyTunnelError(err)
			}
			writeOperationEventBestEffort(
				"tunnel.start",
				operationStatus,
				config,
				"",
				"",
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(config, cwd, "tunnel-start")
			}
		}()

		providerName, _ := cmd.Flags().GetString("provider")
		targetFlag, _ := cmd.Flags().GetString("url")
		noTLSVerify, _ := cmd.Flags().GetBool("no-tls-verify")
		planOnly, _ := cmd.Flags().GetBool("plan")

		targetURL, err := resolveTunnelTarget(config, targetFlag, args)
		if err != nil {
			return err
		}

		provider, err := tunnelDeps.NewProvider(engine.ProviderRef{
			Kind: engine.ProviderKindTunnel,
			Name: providerName,
		})
		if err != nil {
			return err
		}

		plan, err := provider.BuildStartPlan(tunnel.StartOptions{
			TargetURL:   targetURL,
			NoTLSVerify: noTLSVerify,
		})
		if err != nil {
			return err
		}

		if planOnly {
			fmt.Fprintln(cmd.OutOrStdout(), "Tunnel Plan")
			fmt.Fprintf(cmd.OutOrStdout(), "provider: %s\n", provider.Name())
			fmt.Fprintf(cmd.OutOrStdout(), "target: %s\n", targetURL)
			fmt.Fprintf(cmd.OutOrStdout(), "command: %s\n", plan.CommandString())
			operationStatus = engine.OperationStatusPlan
			operationMessage = "tunnel plan generated"
			return nil
		}

		process := exec.Command(plan.Binary, plan.Args...)
		process.Env = append(os.Environ(), plan.Env...)
		process.Stdin = cmd.InOrStdin()
		process.Stdout = cmd.OutOrStdout()
		process.Stderr = cmd.ErrOrStderr()

		pterm.Info.Printf("Starting tunnel provider '%s' to %s. Press Ctrl+C to stop.\n", provider.Name(), targetURL)
		if err := tunnelDeps.RunCommand(process); err != nil {
			return fmt.Errorf("tunnel provider %s failed: %w", provider.Name(), err)
		}

		operationStatus = engine.OperationStatusSuccess
		operationMessage = "tunnel session completed"
		pterm.Success.Println("Tunnel session completed.")
		return nil
	},
}

func init() {
	tunnelStartCmd.Flags().String("provider", "cloudflare", "Tunnel provider (cloudflare)")
	tunnelStartCmd.Flags().String("url", "", "Target URL to expose (defaults to https://<domain> from config)")
	tunnelStartCmd.Flags().Bool("no-tls-verify", true, "Disable TLS verification against target URL")
	tunnelStartCmd.Flags().Bool("plan", false, "Print tunnel execution plan and exit")
	tunnelCmd.AddCommand(tunnelStartCmd)
}

func resolveTunnelTarget(config engine.Config, targetFlag string, args []string) (string, error) {
	targetArg := ""
	if len(args) > 0 {
		targetArg = strings.TrimSpace(args[0])
	}
	trimmedFlag := strings.TrimSpace(targetFlag)
	if trimmedFlag != "" && targetArg != "" {
		return "", fmt.Errorf("specify either positional [url] or --url, not both")
	}

	switch {
	case trimmedFlag != "":
		return trimmedFlag, nil
	case targetArg != "":
		return targetArg, nil
	}

	domain := strings.TrimSpace(config.Domain)
	if domain == "" {
		return "", fmt.Errorf("domain is required to infer tunnel URL; set domain in .govard.yml or pass --url")
	}
	if strings.HasPrefix(strings.ToLower(domain), "http://") || strings.HasPrefix(strings.ToLower(domain), "https://") {
		return domain, nil
	}
	return "https://" + domain, nil
}

func classifyTunnelError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "specify either positional"),
		strings.Contains(message, "target url"),
		strings.Contains(message, "domain is required"),
		strings.Contains(message, "unsupported"),
		strings.Contains(message, "provider"):
		return "validation"
	default:
		return "runtime"
	}
}

// SetTunnelDependenciesForTest swaps tunnel command dependencies and returns a restore callback.
func SetTunnelDependenciesForTest(deps TunnelDependenciesForTest) func() {
	previous := tunnelDeps
	if deps.NewProvider != nil {
		tunnelDeps.NewProvider = deps.NewProvider
	} else {
		tunnelDeps.NewProvider = tunnel.NewProvider
	}
	if deps.RunCommand != nil {
		tunnelDeps.RunCommand = deps.RunCommand
	} else {
		tunnelDeps.RunCommand = func(command *exec.Cmd) error { return command.Run() }
	}
	return func() {
		tunnelDeps = previous
	}
}
