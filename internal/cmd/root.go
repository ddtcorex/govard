package cmd

import (
	"govard/internal/conventions"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"govard/internal/engine"
	"govard/internal/ui"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var Version = "1.59.0"

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
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			pterm.EnableDebugMessages()
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
		} else {
			// Write background logs to temp file for audits/diagnostics
			logFile := filepath.Join(os.TempDir(), "govard.log")
			if file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, conventions.PublicFilePerm); err == nil {
				slog.SetDefault(slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})))
			} else {
				slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
			}
		}

		if cmd.Name() == "help" {
			ui.PrintBrand(Version)
		}

		// Cleanup stale compose files in background once a day
		engine.AutoCleanupComposeFiles()
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
		pterm.Error.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose structured logging")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(svcCmd)
	rootCmd.AddCommand(upShortcutCmd)
	rootCmd.AddCommand(downShortcutCmd)
	rootCmd.AddCommand(restartShortcutCmd)
	rootCmd.AddCommand(psShortcutCmd)
	rootCmd.AddCommand(logsShortcutCmd)

	// Direct service shortcuts (alias for 'env <service>')
	rootCmd.AddCommand(redisCmd)
	rootCmd.AddCommand(valkeyCmd)
	rootCmd.AddCommand(elasticsearchCmd)
	rootCmd.AddCommand(opensearchCmd)
	rootCmd.AddCommand(varnishCmd)
	rootCmd.AddCommand(rabbitmqCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(desktopCmd)
	rootCmd.AddCommand(domainCmd)

	// Framework & Tooling Shortcuts
	initFrameworkCommands()
	initVSCodeCommands()

	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(extensionsCmd)
	initProjectCommands()
	rootCmd.AddCommand(projectCmd)
	registerProjectCustomCommands()
	rootCmd.AddCommand(customCmd)
	rootCmd.AddCommand(selfUpdateCmd)
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(blueprintCmd)
	rootCmd.AddCommand(tunnelCmd)
	rootCmd.AddCommand(versionCmd)
}
