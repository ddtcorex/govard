package cmd

import (
	"fmt"
	"govard/internal/cli"
	"govard/internal/conventions"
	"os"
	"os/exec"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var varnishCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "varnish [command]",
	Short: "Control the varnish service",
	Long: `Interact with the Varnish service.
Supports custom utility commands (log, ban <pattern>, stats) and standard Docker Compose maintenance commands (ps, logs, stop, start, etc.).
Note: 'log' streams the Varnish request log; 'logs' shows the container logs via compose. 'ban' requires a pattern argument.`,
	Example: `  # View varnish logs
  govard varnish log

  # Ban a pattern
  govard varnish ban /.*

  # Check varnish status
  govard varnish ps`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
				return cmd.Help()
			}
			if isComposeMaintenanceCommand(args[0]) {
				return proxyServiceToCompose(cmd, "varnish", args)
			}
		}

		config := loadConfig()
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.VarnishSuffix)

		if len(args) == 0 {
			return cmd.Help()
		}

		subcommand := args[0]
		switch subcommand {
		case "log":
			pterm.Info.Println("Streaming Varnish logs...")
			return runVarnishCmd(containerName, []string{"varnishlog"})
		case "stats":
			return runVarnishCmd(containerName, []string{"varnishstat"})
		case "ban":
			if len(args) < 2 {
				return &cli.UsageError{Err: fmt.Errorf("usage: govard varnish ban <pattern> (example: govard varnish ban /.*)")}
			}
			pattern := args[1]
			pterm.Info.Printf("Banning pattern: %s\n", pattern)
			// varnishadm ban "req.url ~ /.*"
			banCmd := fmt.Sprintf("req.url ~ %s", pattern)
			if err := runVarnishCmd(containerName, []string{"varnishadm", "ban", banCmd}); err != nil {
				return err
			}
			pterm.Success.Println("Ban command sent to Varnish")
			return nil
		default:
			return fmt.Errorf("unknown varnish subcommand: %s", subcommand)
		}
	},
}

func runVarnishCmd(containerName string, args []string) error {
	if err := ensureContainerReadyForExec(containerName, "Varnish"); err != nil {
		return err
	}

	dockerArgs := dockerExecBaseArgs()
	dockerArgs = append(dockerArgs, containerName)
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if stateErr := ensureContainerReadyForExec(containerName, "Varnish"); stateErr != nil {
			return fmt.Errorf("varnish command failed: %w", stateErr)
		}
		return fmt.Errorf("varnish command failed: %w", err)
	}
	return nil
}

func init() {
	// Dual-registered under root and env: pin standard help so --help never
	// depends on init order resolving the parent to the rebranded env command.
	varnishCmd.SetHelpFunc(standardHelpFunc())
}
