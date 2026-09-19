package cmd

import (
	"govard/internal/runtime"
)

import "github.com/spf13/cobra"

var upShortcutCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:     "up [flags] [SERVICE...]",
	Short:   "Shortcut for `govard env up`",
	Long:    "Start the development environment. This is a root shortcut for `govard env up` and supports the same Govard-specific flags. Naming services starts only those services.",
	Example: "  govard up --quickstart\n  govard up --profile staging --pull\n  govard up php",
	RunE:    runUpCommand,
}

var downShortcutCmd = newEnvShortcutCommand(
	"down",
	"Tear down the project environment",
	"Shortcut for `govard env down` with Docker Compose flags passed through as-is.",
	"  govard down\n  govard down -v",
)

var restartShortcutCmd = newEnvShortcutCommand(
	"restart",
	"Restart the project environment",
	"Shortcut for `govard env restart` with Docker Compose flags passed through as-is.",
	"",
)

var psShortcutCmd = newEnvShortcutCommand(
	"ps",
	"List project containers",
	"Shortcut for `govard env ps` with Docker Compose flags passed through as-is.",
	"",
)

var logsShortcutCmd = newEnvShortcutCommand(
	"logs",
	"Stream project logs",
	"Shortcut for `govard env logs` with Docker Compose flags passed through as-is. Supports the Govard-only --errors flag to stream error lines.",
	"  govard logs -f\n  govard logs --errors",
)

func newEnvShortcutCommand(use string, short string, long string, example string) *cobra.Command {
	return &cobra.Command{
		Annotations: map[string]string{
			runtime.AnnotationRequires: string(runtime.CapDocker),
		},
		Use:                use + " [args]",
		Short:              short,
		Long:               long,
		Example:            example,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shouldShowShortcutHelp(args) {
				return cmd.Help()
			}
			return proxyEnvToCompose(cmd, append([]string{use}, args...))
		},
	}
}

func shouldShowShortcutHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

func init() {
	addUpFlags(upShortcutCmd)
}
