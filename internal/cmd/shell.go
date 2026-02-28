package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var noTty bool

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Enter the application container",
	Run: func(cmd *cobra.Command, args []string) {
		config := loadConfig()
		containerName := fmt.Sprintf("%s-php-1", config.ProjectName)

		user := "www-data"

		execArgs := []string{"exec"}
		if !noTty {
			execArgs = append(execArgs, "-it")
		}
		execArgs = append(execArgs, "-u", user, containerName, "bash")

		c := exec.Command("docker", execArgs...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr

		// Try bash, fallback to sh if it fails
		if err := c.Run(); err != nil {
			execArgs[len(execArgs)-1] = "sh"
			c = exec.Command("docker", execArgs...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			_ = c.Run()
		}
	},
}

func init() {
	shellCmd.Flags().BoolVar(&noTty, "no-tty", false, "Disable TTY for non-interactive environments (CI)")
}
