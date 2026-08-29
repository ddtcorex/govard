package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:     "shell [-c <command>] [--] [args...]",
	Aliases: []string{"sh"},
	Short:   "Enter the application container",
	Long: `Enter the application container interactively, or execute a command inside it.

Without arguments, starts an interactive bash session.

With -c/--command, executes the given command string via bash -c:

  govard shell -c "printenv | grep APP_ENV"
  govard shell --command "php artisan --version"

Without -c, remaining arguments are joined and executed via bash -c:

  govard shell php artisan --version
  govard shell ls -la /var/www

Pass -- to disambiguate flags:

  govard shell -- -c "echo hi"`,
	Example: `  govard shell
  govard shell -c "echo hi"
  govard shell -- -c "echo hi"
  govard shell echo hi
  govard shell ls -la
  govard shell -c "printenv | grep -E 'APP_ENV|DATABASE_URL'"`,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Strip leading -- used to separate govard flags from shell command.
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
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
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				if code == 126 || code == 127 {
					err = RunInContainerAt(containerName, user, workdir, "sh", []string{"-c", bashCmd})
				}
			}
		} else if args[0] == "-c" || args[0] == "--command" || strings.HasPrefix(args[0], "--command=") || strings.HasPrefix(args[0], "-c=") {
			var commandStr string
			if args[0] == "-c" || args[0] == "--command" {
				if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
					return fmt.Errorf("flag -c requires an argument")
				}
				if len(args) > 2 {
					commandStr = args[1] + " " + strings.Join(args[2:], " ")
				} else {
					commandStr = args[1]
				}
			} else if strings.HasPrefix(args[0], "--command=") {
				commandStr = strings.TrimPrefix(args[0], "--command=")
				if len(args) > 1 {
					commandStr += " " + strings.Join(args[1:], " ")
				}
			} else { // -c=
				commandStr = strings.TrimPrefix(args[0], "-c=")
				if len(args) > 1 {
					commandStr += " " + strings.Join(args[1:], " ")
				}
			}
			if strings.TrimSpace(commandStr) == "" {
				return fmt.Errorf("flag -c requires an argument")
			}
			err = RunInContainerAt(containerName, user, workdir, "bash", []string{"-c", commandStr})
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				if code == 126 || code == 127 {
					err = RunInContainerAt(containerName, user, workdir, "sh", []string{"-c", commandStr})
				}
			}
		} else {
			commandStr := strings.Join(args, " ")
			err = RunInContainerAt(containerName, user, workdir, "bash", []string{"-c", commandStr})
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				if code == 126 || code == 127 {
					err = RunInContainerAt(containerName, user, workdir, "sh", []string{"-c", commandStr})
				}
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
