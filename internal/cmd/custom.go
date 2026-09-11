package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var customCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "custom",
	Short: "Run custom commands from project and global plugin directories",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var customListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available custom commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		commands, err := discoverCustomCommands()
		if err != nil {
			return err
		}
		if len(commands) == 0 {
			pterm.Info.Println("No custom commands found in project or global plugin directories")
			return nil
		}

		pterm.Success.Println("Available custom commands:")
		for _, item := range commands {
			pterm.Println(" - " + item.Name)
		}
		return nil
	},
}

func registerProjectCustomCommands() {
	customCmd.AddCommand(customListCmd)

	commands, err := discoverCustomCommands()
	if err != nil {
		return
	}

	for _, item := range commands {
		projectCommand := item
		command := &cobra.Command{
			Use:                projectCommand.Name,
			Short:              fmt.Sprintf("Run %s", projectCommand.Path),
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runProjectCustomCommand(projectCommand, args)
			},
		}
		customCmd.AddCommand(command)
	}
}

func discoverCustomCommands() ([]engine.ProjectCommand, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return engine.DiscoverMergedCommands(wd)
}

func runProjectCustomCommand(projectCommand engine.ProjectCommand, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := os.Stat(projectCommand.Path)
	if err != nil {
		return fmt.Errorf("cannot access custom command %s: %w", projectCommand.Name, err)
	}

	var command *exec.Cmd
	if info.Mode()&0111 != 0 {
		command = exec.Command(projectCommand.Path, args...)
	} else {
		withScript := append([]string{projectCommand.Path}, args...)
		command = exec.Command("bash", withScript...)
	}

	command.Dir = wd
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(),
		"GOVARD_PROJECT_ROOT="+wd,
		"GOVARD_CUSTOM_COMMAND="+projectCommand.Name,
	)
	if err := command.Run(); err != nil {
		return fmt.Errorf("custom command %s failed: %w", projectCommand.Name, err)
	}
	return nil
}
