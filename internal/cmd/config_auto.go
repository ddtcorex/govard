package cmd

import (
	"fmt"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var configAutoCmd = &cobra.Command{
	Use:   "auto",
	Short: "Auto-configure framework runtime files",
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Govard Auto-Configuration")

		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		if err := applyFrameworkAutoConfiguration(config); err != nil {
			return fmt.Errorf("configuration failed: %w", err)
		}
		return nil
	},
}

func applyFrameworkAutoConfiguration(config engine.Config) error {
	switch config.Framework {
	case "magento2":
		return engine.ConfigureMagento(config.ProjectName, config)
	default:
		pterm.Warning.Printf(
			"Auto configuration is not supported for framework %q yet.\n",
			config.Framework,
		)
		return nil
	}
}

func init() {
	configCmd.AddCommand(configAutoCmd)
}
