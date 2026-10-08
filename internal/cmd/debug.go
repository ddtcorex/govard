package cmd

import (
	"fmt"
	"os/exec"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/conventions"
	"govard/internal/runtime"
)

var debugCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:     "debug [on|off|status|shell] [args...]",
	Aliases: []string{"dbg"},
	Short:   "Manage Xdebug for the current environment",
	Long: `Toggle Xdebug on or off, check its status, or open a debug shell.
When run without subcommands, it opens a debug shell, which requires Xdebug
to be enabled first ('govard debug on') or the shell refuses to start.
Changes to on/off will trigger an environment update.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebugShell(cmd, args)
	},
}

var debugOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable Xdebug",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadWritableConfig()
		if err != nil {
			return err
		}
		if config.Stack.Features.Xdebug {
			pterm.Info.Println("Xdebug is already enabled")
			return nil
		}
		config.Stack.Features.Xdebug = true
		if err := saveConfig(config); err != nil {
			return err
		}
		pterm.Success.Println("Xdebug enabled in .govard.yml. Running 'govard env up' to apply...")
		runUp()
		return nil
	},
}

var debugOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable Xdebug",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadWritableConfig()
		if err != nil {
			return err
		}
		if !config.Stack.Features.Xdebug {
			pterm.Info.Println("Xdebug is already disabled")
			return nil
		}
		config.Stack.Features.Xdebug = false
		if err := saveConfig(config); err != nil {
			return err
		}
		pterm.Success.Println("Xdebug disabled in .govard.yml. Running 'govard env up' to apply...")
		// php-debug is no longer in the compose file; without this it keeps
		// running as an orphan after the switch-off.
		_ = upCmd.Flags().Set("remove-orphans", "true")
		runUp()
		return nil
	},
}

var debugStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Xdebug status",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		status := "disabled"
		if config.Stack.Features.Xdebug {
			status = "enabled"
		}
		pterm.Info.Printf("Xdebug is currently %s\n", status)
		pterm.Info.Printf("IDE Server Name: %s-docker\n", config.ProjectName)
		return nil
	},
}

var debugShellCmd = &cobra.Command{
	Use:                "shell",
	Short:              "Open a debug shell",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return debugCmd.RunE(cmd, args)
	},
}

// debugShellInvocation builds the argv that opens the debug container's shell,
// keeping the Xdebug exports the IDE session keys on.
//
// A bare `govard debug` is an interactive session: nothing is forwarded, and the
// caller must exec it with stdin attached. A passthrough invocation is a
// one-shot wrapper and keeps its stdin detached, like `govard sh -c`.
//
// The passthrough must not flatten its args into a single string. Joining them
// made `govard debug shell -c "php -r 'echo 1;'"` reach bash as three separate
// words, so bash ran `php` with no arguments and the command silently did
// nothing. The `bash -c SCRIPT NAME ARGS...` idiom hands them back through
// "$@" with their boundaries and quoting intact.
func debugShellInvocation(projectName string, shell string, args []string) (string, []string) {
	exports := fmt.Sprintf("export XDEBUG_SESSION=PHPSTORM; export PHP_IDE_CONFIG=\"serverName=%s-docker\";", projectName)
	if len(args) == 0 {
		// Colored PS1 trick, matching the `govard sh` session.
		coloredPS1 := "\\[\\033[01;36m\\]\\u@\\h\\[\\033[00m\\]:\\w\\$ "
		return shell, []string{"-c", fmt.Sprintf("%s export PS1='%s'; exec %s", exports, coloredPS1, shell)}
	}
	return shell, append([]string{"-c", fmt.Sprintf("%s exec %s \"$@\"", exports, shell), shell}, args...)
}

// DebugShellInvocationForTest exposes debugShellInvocation for tests in /tests.
func DebugShellInvocationForTest(projectName string, args []string) (string, []string) {
	return debugShellInvocation(projectName, "bash", args)
}

func runDebugShell(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		return cmd.Help()
	}
	config, err := loadFullConfig()
	if err != nil {
		return err
	}
	if !config.Stack.Features.Xdebug {
		return fmt.Errorf("xdebug is disabled. Enable it with 'govard debug on'")
	}
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPDebugSuffix)
	user := ResolveProjectExecUser(config, conventions.UserWWWData)

	pterm.Info.Printf("IDE Server Name: %s-docker\n", config.ProjectName)

	// Only the bare form is a session. `docker exec` without -i hands bash an
	// already-closed stdin, so it would exit on the spot and the shell would
	// never open; a passthrough is a one-shot and keeps its stdin detached.
	interactive := len(args) == 0
	binary, argv := debugShellInvocation(config.ProjectName, "bash", args)
	err = RunInContainerAt(containerName, user, conventions.DefaultWorkDir, binary, argv, interactive)

	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == 126 || code == 127 {
			// Fallback to sh if bash is not available/executable. The exports
			// live in the script, so sh keeps them too.
			shBinary, shArgv := debugShellInvocation(config.ProjectName, "sh", args)
			err = RunInContainerAt(containerName, user, conventions.DefaultWorkDir, shBinary, shArgv, interactive)
		}
	}

	return err
}

func init() {
	debugCmd.AddCommand(debugOnCmd)
	debugCmd.AddCommand(debugOffCmd)
	debugCmd.AddCommand(debugStatusCmd)
	debugCmd.AddCommand(debugShellCmd)
}
