package cmd

import (
	"context"
	"fmt"
	"govard/internal/conventions"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"govard/internal/cli"
	"govard/internal/engine"
	_ "govard/internal/frameworks" // registers framework detection/config data via init()
	"govard/internal/ui"
	"govard/internal/updater"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var Version = "dev"

var verbose bool
var errorJSON bool

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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

		return gateError(cmd)
	},
}

var versionCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "version",
	Short: "Print the version number of Govard",
	Run: func(cmd *cobra.Command, args []string) {
		ui.PrintBrand(Version)
		if notice := versionChannelNotice(updater.GetChannel()); notice != "" {
			pterm.Info.Println(notice)
		}
	},
}

func versionChannelNotice(channel string) string {
	if channel == updater.ChannelStable {
		return ""
	}
	return fmt.Sprintf("Update channel: %s", channel)
}

// VersionChannelNoticeForTest exposes versionChannelNotice for tests in /tests.
func VersionChannelNoticeForTest(channel string) string {
	return versionChannelNotice(channel)
}

// GenCompletionForTest exposes cobra completion generation for tests
// and release packaging.
func GenCompletionForTest(shell string) (string, error) {
	var buf strings.Builder
	var err error
	switch shell {
	case "bash":
		err = rootCmd.GenBashCompletion(&buf)
	case "zsh":
		err = rootCmd.GenZshCompletion(&buf)
	case "fish":
		err = rootCmd.GenFishCompletion(&buf, true)
	case "powershell":
		err = rootCmd.GenPowerShellCompletion(&buf)
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Execute() {
	// A Ctrl-C has to reach the command rather than kill the process under it.
	// The deploy's own failure path is what releases a pre-publish lock, keeps a
	// post-publish one, writes the release record and prints the sentence that
	// says how to continue; a process killed mid-step leaves the target holding a
	// lock — and, past `maintenance:enable`, a maintenance flag — that nothing
	// explains and only `deploy unlock` can clear.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SetContext rather than ExecuteContext: the error envelope below needs the
	// command ExecuteC reports, and ExecuteContext does not return it.
	rootCmd.SetContext(ctx)
	executed, err := rootCmd.ExecuteC()
	if err == nil {
		return
	}
	// ExecuteC reports the executed command even when it failed before the
	// pre-run gate (for example on argument validation).
	command := rootCmd.CommandPath()
	if executed != nil {
		command = executed.CommandPath()
	}
	err = asUsageIfArgumentError(err)
	if errorJSON {
		if raw, marshalErr := cli.NewErrorEnvelope(command, err).JSON(); marshalErr == nil {
			fmt.Println(string(raw))
			os.Exit(cli.Code(err))
		}
	}
	// Errors belong on stderr. stdout carries machine-readable output — the
	// deploy result document under `--json`, the error envelope under
	// `--error-json` — and a decorated error line printed into the same stream
	// makes that output unparseable.
	pterm.Error.WithWriter(os.Stderr).Println(err)
	os.Exit(cli.Code(err))
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose structured logging")
	rootCmd.PersistentFlags().BoolVar(&errorJSON, "error-json", false, "Print failures as a machine-readable JSON envelope on stdout")

	// Flag and argument errors are usage errors, not execution failures.
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &cli.UsageError{Err: err}
	})

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(frontendCmd)
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
	rootCmd.AddCommand(newAuditCommand(defaultAuditCommandDependencies()))
	rootCmd.AddCommand(tunnelCmd)
	rootCmd.AddCommand(trustCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(capabilitiesCmd)
}

// asUsageIfArgumentError maps cobra's own argument-validation errors to the
// usage exit code, so callers can tell "you called it wrong" from "it failed".
// Cobra reports these as plain errors; the markers below are its generated
// message shapes.
func asUsageIfArgumentError(err error) error {
	message := err.Error()
	for _, marker := range []string{"arg(s), received", "unknown command", "requires at least", "accepts between"} {
		if strings.Contains(message, marker) {
			return &cli.UsageError{Err: err}
		}
	}
	return err
}
