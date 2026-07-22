package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

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
Govard automatically routes commands to the correct container (usually PHP) and
executes them as the appropriate user (e.g., conventions.UserWWWData).

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

var frameworkCommands = []FrameworkCommand{
	{
		Name:        "magento",
		Short:       "Run Magento CLI commands",
		Frameworks:  []string{"magento2", "mageos"},
		Binary:      "php",
		PrependArgs: []string{"bin/magento"},
		DefaultUser: "",
	},
	{
		Name:        "artisan",
		Short:       "Run Laravel Artisan commands",
		Frameworks:  []string{"laravel"},
		Binary:      "php",
		PrependArgs: []string{"artisan"},
		DefaultUser: "",
	},
	{
		Name:        "magerun",
		Aliases:     []string{"mr"},
		Short:       "Run n98-magerun commands",
		Frameworks:  []string{"magento1", "magento2", "mageos", "openmage"},
		Binary:      "n98-magerun",
		DefaultUser: "",
	},
	{
		Name:        "drush",
		Short:       "Run Drupal Drush commands",
		Frameworks:  []string{"drupal"},
		Binary:      "drush",
		DefaultUser: "",
	},
	{
		Name:        "symfony",
		Short:       "Run Symfony CLI commands",
		Frameworks:  []string{"symfony"},
		Binary:      "php",
		PrependArgs: []string{"bin/console"},
		DefaultUser: "",
	},
	{
		Name:        "shopware",
		Short:       "Run Shopware CLI commands",
		Frameworks:  []string{"shopware"},
		Binary:      "bin/console",
		DefaultUser: "",
	},
	{
		Name:        "cake",
		Short:       "Run CakePHP CLI commands",
		Frameworks:  []string{"cakephp"},
		Binary:      "bin/cake",
		DefaultUser: "",
	},
	{
		Name:        "prestashop",
		Short:       "Run PrestaShop CLI commands (Symfony console)",
		Frameworks:  []string{"prestashop"},
		Binary:      "php",
		PrependArgs: []string{"bin/console"},
		DefaultUser: "",
	},
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
		Name:        "wp",
		Short:       "Run WordPress CLI commands",
		Frameworks:  []string{"wordpress"},
		Binary:      "wp",
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

				targetExec := resolveToolExecution(config, target.Binary, target.DefaultUser)
				return RunInContainerAt(targetExec.ContainerName, targetExec.User, targetExec.Workdir, target.Binary, append(target.PrependArgs, args...))
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

func ResolveProjectExecUser(config engine.Config, fallback string) string {
	return config.ResolveProjectExecUser(fallback)
}

func resolveToolExecution(config engine.Config, binary string, defaultUser string) commandExecutionTarget {
	serviceName := engine.ResolveFrameworkAppService(config.Framework)
	workdir := engine.ResolveFrameworkAppWorkdir(config.Framework)
	user := defaultUser

	if engine.FrameworkUsesNodeRuntime(config.Framework) {
		return commandExecutionTarget{
			ContainerName: fmt.Sprintf("%s-%s%s", config.ProjectName, serviceName, conventions.ReplicaSuffix),
			Workdir:       workdir,
			User:          user,
		}
	}

	if engine.IsMagento2Family(config.Framework) && (binary == "php" || binary == "composer" ||
		binary == "npm" || binary == "yarn" || binary == "npx" ||
		binary == "pnpm" || binary == "grunt") {
		user = config.ResolveProjectExecUser(conventions.UserWWWData)
	} else if user == "" {
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
