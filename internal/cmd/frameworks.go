package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/spf13/cobra"
)

type FrameworkCommand struct {
	Name        string
	Aliases     []string
	Short       string
	Frameworks  []string // empty means all
	Binary      string
	PrependArgs []string
	DefaultUser string
}

type commandExecutionTarget struct {
	ContainerName string
	Workdir       string
	User          string
}

var toolCmd = &cobra.Command{
	Use:   "tool [command]",
	Short: "Run framework/tooling commands inside project containers",
	Long: `Run framework CLIs and common package manager commands directly inside the project containers.
This eliminates the need to install PHP, Composer, or Node.js on your host machine.
Govard routes PHP tools to the application container. Node package commands
run inside the application container for Node-runtime frameworks (Next.js,
Emdash); for other frameworks they run in a one-shot Node container matching
stack.node_version, since those application images no longer bundle Node.

Case Studies:
- Clean Workspace: Run 'govard tool magento setup:upgrade' without needing PHP/MySQL on your laptop.
- Unified Workflow: Use the same command regardless of which PHP version the project requires.
- Package Management: Use 'govard tool composer install' to ensure dependencies match the container environment.`,
	Example: `  # Run Magento CLI
  govard tool magento cache:flush

  # Install a composer package
  govard tool composer require monolog/monolog

  # Run a PHP script or vendor binary directly (e.g. for editor integrations)
  govard tool php vendor/bin/phpstan analyze

  # Run npm install
  govard tool npm install`,
}

var genericToolCommands = []FrameworkCommand{
	{
		Name:        "composer",
		Short:       "Run composer commands",
		Binary:      "composer",
		DefaultUser: "",
	},
	{
		Name:        "php",
		Short:       "Run the php CLI directly",
		Binary:      "php",
		DefaultUser: "",
	},
	{
		Name:        "npm",
		Short:       "Run npm commands",
		Binary:      "npm",
		DefaultUser: "",
	},
	{
		Name:        "yarn",
		Short:       "Run yarn commands",
		Binary:      "yarn",
		DefaultUser: "",
	},
	{
		Name:        "npx",
		Short:       "Run npx commands",
		Binary:      "npx",
		DefaultUser: "",
	},
	{
		Name:        "pnpm",
		Short:       "Run pnpm commands",
		Binary:      "pnpm",
		DefaultUser: "",
	},
	{
		Name:        "grunt",
		Short:       "Run grunt commands",
		Binary:      "grunt",
		DefaultUser: "",
	},
}

var frameworkCommands = append(frameworkToolCommands(), genericToolCommands...)

func frameworkToolCommands() []FrameworkCommand {
	byName := make(map[string]int)
	var commands []FrameworkCommand
	for _, definition := range frameworks.All() {
		for _, declaration := range definition.ToolCommands {
			index, exists := byName[declaration.Name]
			if !exists {
				byName[declaration.Name] = len(commands)
				commands = append(commands, FrameworkCommand{
					Name:        declaration.Name,
					Aliases:     append([]string(nil), declaration.Aliases...),
					Short:       declaration.Short,
					Binary:      declaration.Binary,
					PrependArgs: append([]string(nil), declaration.PrependArgs...),
					DefaultUser: declaration.DefaultUser,
				})
				index = len(commands) - 1
			}
			commands[index].Frameworks = append(commands[index].Frameworks, definition.Name)
		}
	}
	for index := range commands {
		sort.Strings(commands[index].Frameworks)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands
}

// FrameworkToolCommandsForTest exposes only framework-owned commands; generic
// Composer/PHP/Node tools deliberately remain outside the registry.
func FrameworkToolCommandsForTest() []FrameworkCommand {
	return frameworkToolCommands()
}

func initFrameworkCommands() {
	for _, fc := range frameworkCommands {
		usage := fmt.Sprintf("%s [args]", fc.Name)
		longDesc := fc.Short
		if len(fc.Frameworks) > 0 {
			longDesc = fmt.Sprintf("%s (Requires %s project)", fc.Short, strings.Join(fc.Frameworks, "/"))
		}
		cmd := &cobra.Command{
			Use:                usage,
			Aliases:            fc.Aliases,
			Short:              fc.Short,
			Long:               longDesc,
			DisableFlagParsing: true,
			RunE: func(c *cobra.Command, args []string) error {
				// Find which command we are running
				name := c.Name()
				var target FrameworkCommand
				foundTarget := false
				for _, tc := range frameworkCommands {
					if tc.Name == name {
						target = tc
						foundTarget = true
					} else {
						for _, alias := range tc.Aliases {
							if alias == name {
								target = tc
								foundTarget = true
								break
							}
						}
					}
					if foundTarget {
						break
					}
				}

				config := loadConfig()

				// Validate framework if required
				if len(target.Frameworks) > 0 {
					frameworkFound := false
					for _, f := range target.Frameworks {
						if f == config.Framework {
							frameworkFound = true
							break
						}
					}
					if !frameworkFound {
						return fmt.Errorf("the '%s' command is only available for %s projects (current: %s)", name, strings.Join(target.Frameworks, "/"), config.Framework)
					}
				}

				commandArgs := append(target.PrependArgs, args...)
				if isNodeTool(target.Binary) && !engine.FrameworkUsesNodeRuntime(config.Framework) {
					return RunNodeTool(config, target.Binary, commandArgs)
				}

				targetExec := resolveToolExecution(config, target.Binary, target.DefaultUser)
				return RunInContainerAt(targetExec.ContainerName, targetExec.User, targetExec.Workdir, target.Binary, commandArgs)
			},
		}
		toolCmd.AddCommand(cmd)
	}
	rootCmd.AddCommand(toolCmd)
}

func RunInContainer(containerName string, user string, binary string, args []string) error {
	return RunInContainerAt(containerName, user, conventions.DefaultWorkDir, binary, args)
}

func RunInContainerAt(containerName string, user string, workdir string, binary string, args []string) error {
	dockerArgs := []string{"exec"}
	if stdinIsTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	} else {
		dockerArgs = append(dockerArgs, "-i")
	}
	if user != "" {
		dockerArgs = append(dockerArgs, "-u", user)
	}
	if strings.TrimSpace(workdir) == "" {
		workdir = conventions.DefaultWorkDir
	}
	dockerArgs = append(dockerArgs, "-w", workdir, containerName, binary)
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// RunNodeTool runs package tooling in a one-shot standalone Node image.
// Only used for frameworks whose application image does not already provide
// Node (PHP, Python) — those images may bundle a different Node major, so
// tooling must not inherit their runtime by accident. Node-runtime
// frameworks (Next.js, Emdash) instead exec into their own running
// container via resolveToolExecution, matching that container's exact
// Node build (libc, native modules, network, environment).
func RunNodeTool(config engine.Config, binary string, args []string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve project directory: %w", err)
	}

	dockerArgs := []string{"run", "--rm"}
	if stdinIsTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	} else {
		dockerArgs = append(dockerArgs, "-i")
	}
	dockerArgs = append(dockerArgs,
		"--user", fmt.Sprintf("%d:%d", config.Stack.UserID, config.Stack.GroupID),
		"-v", projectRoot+":"+conventions.DefaultWorkDir,
		"-w", conventions.DefaultWorkDir,
		"node:"+config.Stack.NodeVersion+"-alpine",
	)

	switch binary {
	case "pnpm":
		dockerArgs = append(dockerArgs, "corepack", "pnpm")
	case "grunt":
		dockerArgs = append(dockerArgs, "npx", "grunt")
	default:
		dockerArgs = append(dockerArgs, binary)
	}
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func isNodeTool(binary string) bool {
	switch binary {
	case "npm", "npx", "yarn", "pnpm", "grunt":
		return true
	default:
		return false
	}
}

func ResolveProjectExecUser(config engine.Config, fallback string) string {
	return config.ResolveProjectExecUser(fallback)
}

func resolveToolExecution(config engine.Config, binary string, defaultUser string) commandExecutionTarget {
	serviceName := engine.ResolveFrameworkAppService(config.Framework)
	workdir := engine.ResolveFrameworkAppWorkdir(config.Framework)
	user := defaultUser

	if engine.FrameworkUsesNodeRuntime(config.Framework) || engine.FrameworkUsesPythonRuntime(config.Framework) {
		return commandExecutionTarget{
			ContainerName: fmt.Sprintf("%s-%s%s", config.ProjectName, serviceName, conventions.ReplicaSuffix),
			Workdir:       workdir,
			User:          user,
		}
	}

	if user == "" {
		user = config.ResolveProjectExecUser(conventions.UserWWWData)
	}

	return commandExecutionTarget{
		ContainerName: fmt.Sprintf("%s-%s%s", config.ProjectName, serviceName, conventions.ReplicaSuffix),
		Workdir:       workdir,
		User:          user,
	}
}

func ResolveToolExecutionForTest(config engine.Config, binary string) (string, string, string) {
	target := resolveToolExecution(config, binary, "")
	return target.ContainerName, target.Workdir, target.User
}

func ValidateFrameworkForCommandForTest(commandName string, config engine.Config) error {
	var target FrameworkCommand
	found := false
	for _, tc := range frameworkCommands {
		if tc.Name == commandName {
			target = tc
			found = true
			break
		}
		for _, alias := range tc.Aliases {
			if alias == commandName {
				target = tc
				found = true
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("command not found")
	}

	if len(target.Frameworks) > 0 {
		frameworkFound := false
		for _, f := range target.Frameworks {
			if f == config.Framework {
				frameworkFound = true
				break
			}
		}
		if !frameworkFound {
			return fmt.Errorf("the '%s' command is only available for %s projects (current: %s)", commandName, strings.Join(target.Frameworks, "/"), config.Framework)
		}
	}

	return nil
}
