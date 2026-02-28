package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"govard/internal/ui"

	"github.com/spf13/cobra"
)

var Version = "1.8.0"

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "govard",
	Short: "Govard: Professional local development orchestrator for PHP & Web projects",
	Long: `Govard is a high-performance orchestrator designed to manage complex containerized environments.
It replaces legacy bash-based tools with a native Go binary, focusing on stability, speed,
and a premium developer experience.

Main Features:
- Zero-config startup for Magento, Laravel, Symfony, Drupal, and more.
- Automated SSL (HTTPS) for all .test domains.
- Deep integration with Xdebug 3.
- Fast file/database synchronization with remote environments.
- Built-in desktop dashboard for visual management.

Documentation: https://github.com/ddtcorex/govard`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: false,
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
		} else {
			// Write background logs to temp file for audits/diagnostics
			logFile := filepath.Join(os.TempDir(), "govard.log")
			if file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
				slog.SetDefault(slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})))
			} else {
				slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
			}
		}

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
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose structured logging")

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
	rootCmd.AddCommand(domainCmd)

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
