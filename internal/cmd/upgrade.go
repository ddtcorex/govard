package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the framework version",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := loadFullConfig()
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		if config.Framework == "" {
			pterm.Warning.Println("No framework configured in .govard.yml.")
			return
		}
		pterm.Info.Printf("Upgrade for %s is not implemented yet.\n", config.Framework)
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

// UpgradeCommand exposes the upgrade command for tests.
func UpgradeCommand() *cobra.Command {
	return upgradeCmd
}
