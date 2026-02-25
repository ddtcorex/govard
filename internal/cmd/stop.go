package cmd

import (
	"fmt"
	"govard/internal/engine"
	"govard/internal/proxy"
	"os"
	"os/exec"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop project containers",
	Long: `Stops all running containers for the current project without removing them.
It also unregisters the project domain from the Govard Proxy and removes host entries.
Use this to pause work and free up CPU/RAM while preserving your local data (volumes).

Case Studies:
- End of Day: Run 'govard stop' to shut down the project before turning off your computer.
- Switching Projects: Stop the current project to avoid port conflicts or resource contention when starting another.
- Battery Saving: Stop the environment when working on non-code tasks to extend laptop battery life.`,
	Example: `  # Stop the environment
  govard stop`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Stopping Govard Environment")

		config := loadConfig()
		cwd, _ := os.Getwd()
		composePath := engine.ComposeFilePath(cwd, config.ProjectName)
		if err := engine.RunHooks(config, engine.HookPreStop, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-stop hooks failed: %w", err)
		}

		c := exec.Command("docker", "compose", "--project-directory", cwd, "-f", composePath, "stop")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
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

		pterm.Success.Println("✅ Environment stopped.")
		return nil
	},
}
