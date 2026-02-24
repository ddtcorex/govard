package cmd

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug [on|off|status|shell]",
	Short: "Toggle Xdebug for the current environment",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		config := loadFullConfig()

		subcommand := "status"
		if len(args) > 0 {
			subcommand = args[0]
		}

		switch subcommand {
		case "on":
			config = loadWritableConfig()
			if config.Stack.Features.Xdebug {
				pterm.Info.Println("Xdebug is already enabled")
				return
			}
			config.Stack.Features.Xdebug = true
			saveConfig(config)
			pterm.Success.Println("Xdebug enabled in govard.yml. Running 'govard up' to apply...")
			runUp()
		case "off":
			config = loadWritableConfig()
			if !config.Stack.Features.Xdebug {
				pterm.Info.Println("Xdebug is already disabled")
				return
			}
			config.Stack.Features.Xdebug = false
			saveConfig(config)
			pterm.Success.Println("Xdebug disabled in govard.yml. Running 'govard up' to apply...")
			runUp()
		case "status":
			status := "disabled"
			if config.Stack.Features.Xdebug {
				status = "enabled"
			}
			pterm.Info.Printf("Xdebug is currently %s\n", status)
		case "shell":
			if !config.Stack.Features.Xdebug {
				pterm.Error.Println("Xdebug is disabled. Enable it with 'govard debug on'.")
				return
			}
			containerName := fmt.Sprintf("%s-php-debug-1", config.ProjectName)
			if err := runInContainer(containerName, resolveProjectExecUser(config, "www-data"), "bash", []string{}); err != nil {
				pterm.Error.Printf("failed to open debug shell: %v\n", err)
			}
		default:
			pterm.Error.Printf("Unknown debug subcommand: %s\n", subcommand)
		}
	},
}
