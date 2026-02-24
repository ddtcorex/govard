package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the framework version",
	Run: func(cmd *cobra.Command, args []string) {
		config := loadFullConfig()
		if config.Recipe == "" {
			pterm.Warning.Println("No recipe configured in .govard.yml.")
			return
		}
		pterm.Info.Printf("Upgrade for %s is not implemented yet.\n", config.Recipe)
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

// UpgradeCommand exposes the upgrade command for tests.
func UpgradeCommand() *cobra.Command {
	return upgradeCmd
}
