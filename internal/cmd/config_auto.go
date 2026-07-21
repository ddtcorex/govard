package cmd

import (
	"fmt"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	runMagento2AutoConfiguration = func(projectName string, config engine.Config, force bool) error {
		return engine.ConfigureMagento(projectName, config, force, nil)
	}
	runMagento1AutoConfiguration = engine.ConfigureMagento1
)

var configAutoCmd = &cobra.Command{
	Use:   "auto",
	Short: "Auto-configure framework runtime files",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Auto-Configuration ")
		fmt.Println()

		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		if err := applyFrameworkAutoConfiguration(cmd, config); err != nil {
			return fmt.Errorf("configuration failed: %w", err)
		}
		return nil
	},
}

func applyFrameworkAutoConfiguration(cmd *cobra.Command, config engine.Config) error {
	switch config.Framework {
	case "magento2", "mageos":
		// Proactively fix search host in DB via CLI (using govard db query)
		if config.Stack.Features.Search || config.Stack.Services.Search != "none" {
			_ = runMagentoSearchHostFixViaCLI(cmd, config)
		}
		return runMagento2AutoConfiguration(config.ProjectName, config, true)
	case "magento1", "openmage":
		return runMagento1AutoConfiguration(config.ProjectName, config)
	case "wordpress":
		return nil
	default:
		pterm.Warning.Printf(
			"Auto configuration is not supported for framework %q yet.\n",
			config.Framework,
		)
		return nil
	}
}

func SetMagento1AutoConfigurationRunnerForTest(fn func(projectName string, config engine.Config) error) func() {
	previous := runMagento1AutoConfiguration
	runMagento1AutoConfiguration = fn
	return func() {
		runMagento1AutoConfiguration = previous
	}
}

func SetMagento2AutoConfigurationRunnerForTest(fn func(projectName string, config engine.Config, force bool) error) func() {
	previous := runMagento2AutoConfiguration
	runMagento2AutoConfiguration = fn
	return func() {
		runMagento2AutoConfiguration = previous
	}
}

func ApplyFrameworkAutoConfigurationForTest(config engine.Config) error {
	return applyFrameworkAutoConfiguration(nil, config)
}

func init() {
	configCmd.AddCommand(configAutoCmd)
}
