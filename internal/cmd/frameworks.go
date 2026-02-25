package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"govard/internal/engine"

	"github.com/spf13/cobra"
)

type RecipeCommand struct {
	Name        string
	Short       string
	Recipe      string // empty means all
	Binary      string
	PrependArgs []string
	DefaultUser string
}

var toolCmd = &cobra.Command{
	Use:   "tool [command]",
	Short: "Run framework/tooling commands inside project containers",
	Long: `Run framework CLIs and common package manager commands directly inside the project containers.
This eliminates the need to install PHP, Composer, or Node.js on your host machine.
Govard automatically routes commands to the correct container (usually PHP) and
executes them as the appropriate user (e.g., www-data).

Case Studies:
- Clean Workspace: Run 'govard tool magento setup:upgrade' without needing PHP/MySQL on your laptop.
- Unified Workflow: Use the same command regardless of which PHP version the project requires.
- Package Management: Use 'govard tool composer install' to ensure dependencies match the container environment.`,
	Example: `  # Run Magento CLI
  govard tool magento cache:flush

  # Install a composer package
  govard tool composer require monolog/monolog

  # Run npm install
  govard tool npm install`,
}

var frameworkCommands = []RecipeCommand{
	{
		Name:        "magento",
		Short:       "Run Magento CLI commands",
		Recipe:      "magento2",
		Binary:      "php",
		PrependArgs: []string{"bin/magento"},
		DefaultUser: "",
	},
	{
		Name:        "artisan",
		Short:       "Run Laravel Artisan commands",
		Recipe:      "laravel",
		Binary:      "php",
		PrependArgs: []string{"artisan"},
		DefaultUser: "",
	},
	{
		Name:        "magerun",
		Short:       "Run n98-magerun commands",
		Recipe:      "magento1",
		Binary:      "n98-magerun.phar",
		DefaultUser: "",
	},
	{
		Name:        "drush",
		Short:       "Run Drupal Drush commands",
		Recipe:      "drupal",
		Binary:      "drush",
		DefaultUser: "",
	},
	{
		Name:        "symfony",
		Short:       "Run Symfony CLI commands",
		Recipe:      "symfony",
		Binary:      "php",
		PrependArgs: []string{"bin/console"},
		DefaultUser: "",
	},
	{
		Name:        "shopware",
		Short:       "Run Shopware CLI commands",
		Recipe:      "shopware",
		Binary:      "bin/console",
		DefaultUser: "",
	},
	{
		Name:        "cake",
		Short:       "Run CakePHP CLI commands",
		Recipe:      "cakephp",
		Binary:      "bin/cake",
		DefaultUser: "",
	},
	{
		Name:        "composer",
		Short:       "Run composer commands",
		Binary:      "composer",
		DefaultUser: "",
	},
	{
		Name:        "wp",
		Short:       "Run WordPress CLI commands",
		Recipe:      "wordpress",
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
		if fc.Recipe != "" {
			longDesc = fmt.Sprintf("%s (Requires %s project)", fc.Short, fc.Recipe)
		}
		cmd := &cobra.Command{
			Use:                usage,
			Short:              fc.Short,
			Long:               longDesc,
			DisableFlagParsing: true,
			RunE: func(c *cobra.Command, args []string) error {
				// Find which command we are running
				name := c.Name()
				var target RecipeCommand
				for _, tc := range frameworkCommands {
					if tc.Name == name {
						target = tc
						break
					}
				}

				config := loadConfig()

				// Validate recipe if required
				if target.Recipe != "" && config.Recipe != target.Recipe {
					return fmt.Errorf("the '%s' command is only available for %s projects (current: %s)", name, target.Recipe, config.Recipe)
				}

				// Determine container and user
				// Most frameworks use 'php' container, node-based use 'app' or 'web'
				containerName := fmt.Sprintf("%s-php-1", config.ProjectName)
				if target.Binary == "npm" || target.Binary == "yarn" || target.Binary == "npx" || target.Binary == "pnpm" {
					// For node commands, we check if we have a node container or use php one (many php images have node)
					// In our blueprints, we usually have node in php or a separate app container
					// For now, default to php container as it's the main workspace
				}

				user := target.DefaultUser
				// Override user if it's magento2
				if config.Recipe == "magento2" && (target.Binary == "php" || target.Binary == "composer" ||
					target.Binary == "npm" || target.Binary == "yarn" || target.Binary == "npx" ||
					target.Binary == "pnpm" || target.Binary == "grunt") {
					user = resolveProjectExecUser(config, "www-data")
				}

				return runInContainer(containerName, user, target.Binary, append(target.PrependArgs, args...))
			},
		}
		toolCmd.AddCommand(cmd)
	}
	rootCmd.AddCommand(toolCmd)
}

func runInContainer(containerName string, user string, binary string, args []string) error {
	dockerArgs := []string{"exec"}
	if stdinIsTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	} else {
		dockerArgs = append(dockerArgs, "-i")
	}
	if user != "" {
		dockerArgs = append(dockerArgs, "-u", user)
	}
	dockerArgs = append(dockerArgs, "-w", "/var/www/html", containerName, binary)
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func resolveProjectExecUser(config engine.Config, fallback string) string {
	if config.Stack.UserID > 0 && config.Stack.GroupID > 0 {
		return fmt.Sprintf("%d:%d", config.Stack.UserID, config.Stack.GroupID)
	}
	return fallback
}
