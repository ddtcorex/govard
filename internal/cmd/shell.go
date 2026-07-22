package cmd

import (
	"fmt"
	"os/exec"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:                "shell",
	Aliases:            []string{"sh"},
	Short:              "Enter the application container",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			return cmd.Help()
		}
		config := loadConfig()
		containerName, workdir, user := resolveShellExecution(config)

		var err error
		if len(args) == 0 {
			// Interactive session with colored PS1 trick (Cyan user@host to match Warden)
			coloredPS1 := "\\[\\033[01;36m\\]\\u@\\h\\[\\033[00m\\]:\\w\\$ "
			bashCmd := fmt.Sprintf("export PS1='%s'; exec bash", coloredPS1)
			err = RunInContainerAt(containerName, user, workdir, "bash", []string{"-c", bashCmd})
		} else {
			err = RunInContainerAt(containerName, user, workdir, "bash", args)
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code == 126 || code == 127 {
				// bash not found or not executable — try sh
				err = RunInContainerAt(containerName, user, workdir, "sh", args)
			}
		}

		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
}

func resolveShellExecution(config engine.Config) (string, string, string) {
	serviceName := engine.ResolveFrameworkAppService(config.Framework)
	workdir := engine.ResolveFrameworkAppWorkdir(config.Framework)
	user := ""
	if !engine.FrameworkUsesNodeRuntime(config.Framework) && !engine.FrameworkUsesPythonRuntime(config.Framework) {
		user = ResolveProjectExecUser(config, conventions.UserWWWData)
	}
	return fmt.Sprintf("%s-%s%s", config.ProjectName, serviceName, conventions.ReplicaSuffix), workdir, user
}
