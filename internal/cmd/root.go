package cmd

import (
	"fmt"
	"govard/internal/ui"
	"os"

	"github.com/spf13/cobra"
)

var Version = "1.3.0"

var rootCmd = &cobra.Command{
	Use:   "govard",
	Short: "Govard is a high-performance local development orchestrator",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if cmd.Name() == "help" {
			ui.PrintBrand(Version)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Govard",
	Run: func(cmd *cobra.Command, args []string) {
		ui.PrintBrand(Version)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(svcCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(desktopCmd)

	// Framework & Tooling Shortcuts
	initFrameworkCommands()

	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(extensionsCmd)
	initProjectsCommands()
	rootCmd.AddCommand(projectsCmd)
	registerProjectCustomCommands()
	rootCmd.AddCommand(customCmd)
	rootCmd.AddCommand(selfUpdateCmd)
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(tunnelCmd)
	rootCmd.AddCommand(versionCmd)
}
