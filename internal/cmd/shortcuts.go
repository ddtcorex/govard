package cmd

import (
	"strconv"
	"strings"

	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

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
			// The global flags govard owns never belong to compose: forwarding them
			// made compose answer "unknown flag: --error-json" instead of govard
			// emitting its envelope.
			forwarded := splitPassthroughGlobals(args).Forwarded
			return proxyEnvToCompose(cmd, append([]string{use}, forwarded...))
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

// passthroughGlobals is what a pass-through command learned from its own argument
// list, and what it should hand on to the wrapped command.
type passthroughGlobals struct {
	// Forwarded is args with govard's global flags removed. Everything else,
	// including Docker Compose's own flags and service names, keeps its order.
	Forwarded []string
	// ErrorJSON and Verbose report the value the caller asked for, when given.
	ErrorJSON bool
	Verbose   bool
	// SawErrorJSON and SawVerbose distinguish "unset" from an explicit false.
	SawErrorJSON bool
	SawVerbose   bool
}

// splitPassthroughGlobals separates the global flags govard owns from the
// arguments that belong to the wrapped command.
//
// The root environment shortcuts set DisableFlagParsing so Docker Compose flags
// reach compose untouched, which also means cobra never parses govard's own
// persistent flags for them: `govard ps --error-json` forwarded the flag and
// compose rejected it, so the documented flag did nothing on four commands.
// Consuming exactly the two flags govard owns keeps the pass-through intact.
// Everything after a `--` is forwarded verbatim, so a caller can still push a
// literal --verbose through to compose.
func splitPassthroughGlobals(args []string) passthroughGlobals {
	var result passthroughGlobals
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result.Forwarded = append(result.Forwarded, args[i:]...)
			return result
		}
		if value, ok := flagValue(arg, "--error-json"); ok {
			if parsed, err := strconv.ParseBool(value); err == nil {
				result.SawErrorJSON, result.ErrorJSON = true, parsed
				continue
			}
		}
		if value, ok := flagValue(arg, "--verbose"); ok {
			if parsed, err := strconv.ParseBool(value); err == nil {
				result.SawVerbose, result.Verbose = true, parsed
				continue
			}
		}
		result.Forwarded = append(result.Forwarded, arg)
	}
	return result
}

// flagValue reports the value of name, both as a bare flag (true) and as
// name=value. A malformed value is left alone so the wrapped command reports it.
func flagValue(arg, name string) (string, bool) {
	if arg == name {
		return "true", true
	}
	if strings.HasPrefix(arg, name+"=") {
		return strings.TrimPrefix(arg, name+"="), true
	}
	return "", false
}

// SplitPassthroughGlobalsForTest exposes the splitter so a test can pin the
// boundary without driving a whole command.
func SplitPassthroughGlobalsForTest(args []string) passthroughGlobals {
	return splitPassthroughGlobals(args)
}
