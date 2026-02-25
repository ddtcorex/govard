package cmd

import (
	"fmt"
	"govard/internal/engine"
	"govard/internal/proxy"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	downRemoveOrphans bool
	downVolumes       bool
	downRMI           string
	downTimeout       int
)

type downOptions struct {
	RemoveOrphans bool
	Volumes       bool
	RMI           string
	Timeout       int
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Tear down project containers and networks",
	Long: `Stops and removes containers, networks, and optionally volumes created by 'govard up'.
This is a destructive operation that completely cleans up the Docker resources
associated with the current project.

Case Studies:
- Fresh Start: Use 'govard down -v' to wipe out all local data (including database and media) for a clean reinstall.
- Troubleshooting: If networks or containers are in a ghost state, 'govard down' ensures they are fully purged.
- Workspace Cleanup: Use 'govard down' when you are finished with a project to free up system resources.`,
	Example: `  # Tear down containers and networks
  govard down

  # Wipe everything including database/media volumes
  govard down --volumes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Tearing Down Govard Environment")

		config := loadConfig()
		cwd, _ := os.Getwd()
		composePath := engine.ComposeFilePath(cwd, config.ProjectName)

		if err := engine.RunHooks(config, engine.HookPreStop, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-stop hooks failed: %w", err)
		}

		composeArgs, err := buildDownComposeArgs(cwd, composePath, config.ProjectName, downOptions{
			RemoveOrphans: downRemoveOrphans,
			Volumes:       downVolumes,
			RMI:           downRMI,
			Timeout:       downTimeout,
		})
		if err != nil {
			return fmt.Errorf("invalid down options: %w", err)
		}

		command := exec.Command("docker", composeArgs...)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("failed to tear down containers: %w", err)
		}

		if config.Domain != "" {
			if err := proxy.UnregisterDomain(config.Domain); err != nil {
				pterm.Warning.Printf("Could not remove proxy route for %s: %v\n", config.Domain, err)
			}
			if err := engine.RemoveHostsEntry(config.Domain); err != nil {
				pterm.Warning.Printf("Could not remove hosts entry for %s: %v\n", config.Domain, err)
			}
		}

		if err := engine.RunHooks(config, engine.HookPostStop, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("post-stop hooks failed: %w", err)
		}

		pterm.Success.Println("✅ Environment torn down.")
		return nil
	},
}

func init() {
	downCmd.Flags().BoolVar(&downRemoveOrphans, "remove-orphans", true, "Remove containers for services not defined in compose file")
	downCmd.Flags().BoolVarP(&downVolumes, "volumes", "v", false, "Remove named and anonymous volumes")
	downCmd.Flags().StringVar(&downRMI, "rmi", "", "Remove images used by services (all|local)")
	downCmd.Flags().IntVarP(&downTimeout, "timeout", "t", 0, "Specify a shutdown timeout in seconds")
}

func buildDownComposeArgs(cwd string, composePath string, projectName string, options downOptions) ([]string, error) {
	normalized, err := normalizeDownOptions(options)
	if err != nil {
		return nil, err
	}

	args := []string{
		"compose",
		"--project-directory",
		cwd,
		"-p",
		projectName,
		"-f",
		composePath,
		"down",
	}

	if normalized.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	if normalized.Volumes {
		args = append(args, "--volumes")
	}
	if normalized.RMI != "" {
		args = append(args, "--rmi", normalized.RMI)
	}
	if normalized.Timeout > 0 {
		args = append(args, "--timeout", strconv.Itoa(normalized.Timeout))
	}

	return args, nil
}

func normalizeDownOptions(options downOptions) (downOptions, error) {
	options.RMI = strings.ToLower(strings.TrimSpace(options.RMI))
	if options.Timeout < 0 {
		return downOptions{}, fmt.Errorf("timeout must be zero or positive")
	}
	switch options.RMI {
	case "", "all", "local":
		return options, nil
	default:
		return downOptions{}, fmt.Errorf("invalid --rmi value %q (allowed: all, local)", options.RMI)
	}
}

func BuildDownComposeArgsForTest(
	cwd string,
	composePath string,
	projectName string,
	removeOrphans bool,
	volumes bool,
	rmi string,
	timeout int,
) ([]string, error) {
	return buildDownComposeArgs(cwd, composePath, projectName, downOptions{
		RemoveOrphans: removeOrphans,
		Volumes:       volumes,
		RMI:           rmi,
		Timeout:       timeout,
	})
}
