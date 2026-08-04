package cmd

import (
	"fmt"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// frameworkLookupForAutoConfigure is swappable in tests so
// ApplyFrameworkAutoConfigurationForTest can exercise a fake AutoConfigure
// closure without registering a throwaway framework in the real,
// process-global frameworks registry.
var frameworkLookupForAutoConfigure = frameworks.Get

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
	def, ok := frameworkLookupForAutoConfigure(config.Framework)
	if !ok || def.AutoConfigure == nil {
		pterm.Warning.Printf(
			"Auto configuration is not supported for framework %q yet.\n",
			config.Framework,
		)
		return nil
	}
	if def.BuildSearchHostFixSQL != nil {
		// The framework provides its SQL while generic command orchestration
		// executes it through the local database command.
		if config.Stack.Features.Search || config.Stack.Services.Search != "none" {
			_ = runFrameworkSearchHostFixViaCLI(cmd, config)
		}
	}
	return def.AutoConfigure(cmd, config)
}

// SetFrameworkLookupForAutoConfigureForTest swaps the framework-lookup used
// by applyFrameworkAutoConfiguration so tests can exercise a fake
// AutoConfigure closure without registering a throwaway framework in the
// real, process-global frameworks registry.
func SetFrameworkLookupForAutoConfigureForTest(fn func(name string) (types.FrameworkDefinition, bool)) func() {
	previous := frameworkLookupForAutoConfigure
	frameworkLookupForAutoConfigure = fn
	return func() {
		frameworkLookupForAutoConfigure = previous
	}
}

func ApplyFrameworkAutoConfigurationForTest(config engine.Config) error {
	return applyFrameworkAutoConfiguration(nil, config)
}

func init() {
	configCmd.AddCommand(configAutoCmd)
}
