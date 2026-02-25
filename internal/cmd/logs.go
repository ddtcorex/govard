package cmd

import (
	"fmt"
	"govard/internal/engine"
	"os"
	"os/exec"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var errorFilter bool

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View project logs",
	Long: `Streams real-time logs from all services in the current project environment.
It automatically aggregates logs from PHP, Web, MySQL, Redis, and other active containers.

Case Studies:
- Real-time Debugging: Watch the log stream while browsing the site to catch errors as they happen.
- Post-Mortem: Check the last 100 lines (default) to see why a container crashed.
- Error Hunting: Use --errors to filter out noise and only show critical failure messages.`,
	Example: `  # Follow all project logs
  govard logs

  # Show only error messages
  govard logs --errors`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Govard Log Stream")
		config := loadConfig()
		cwd, _ := os.Getwd()
		composePath := engine.ComposeFilePath(cwd, config.ProjectName)

		dockerArgs := []string{"compose", "--project-directory", cwd, "-f", composePath, "logs", "-f", "--tail=100"}

		if errorFilter {
			pterm.Info.Println("Filtering for errors...")
			// Simple grep for error-like strings
			filterCommand := fmt.Sprintf(
				"docker compose --project-directory %q -f %q logs -f | grep -iE 'error|critical|fail|exception'",
				cwd,
				composePath,
			)
			c := exec.Command("sh", "-c", filterCommand)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("stream filtered logs: %w", err)
			}
			return nil
		}

		c := exec.Command("docker", dockerArgs...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("stream logs: %w", err)
		}
		return nil
	},
}
