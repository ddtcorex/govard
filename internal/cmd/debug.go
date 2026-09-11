package cmd

import (
	"fmt"
	"os/exec"
	"strings"

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
When run without subcommands, it opens a debug shell.
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
		saveConfig(config)
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
		saveConfig(config)
		pterm.Success.Println("Xdebug disabled in .govard.yml. Running 'govard env up' to apply...")
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

	if len(args) == 0 {
		// Interactive session with colored PS1 trick and Xdebug exports
		coloredPS1 := "\\[\\033[01;36m\\]\\u@\\h\\[\\033[00m\\]:\\w\\$ "
		bashCmd := fmt.Sprintf("export XDEBUG_SESSION=PHPSTORM; export PHP_IDE_CONFIG=\"serverName=%s-docker\"; export PS1='%s'; exec bash", config.ProjectName, coloredPS1)
		err = RunInContainer(containerName, user, "bash", []string{"-c", bashCmd})
	} else {
		// Passthrough commands (e.g. govard debug shell -c "...")
		// We still prefix with Xdebug exports so the command has the debugger active.
		cmdStr := strings.Join(args, " ")
		bashCmd := fmt.Sprintf("export XDEBUG_SESSION=PHPSTORM; export PHP_IDE_CONFIG=\"serverName=%s-docker\"; exec bash %s", config.ProjectName, cmdStr)
		err = RunInContainer(containerName, user, "bash", []string{"-c", bashCmd})
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == 126 || code == 127 {
			// Fallback to sh if bash is not available/executable
			err = RunInContainer(containerName, user, "sh", args)
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
