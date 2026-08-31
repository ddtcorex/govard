package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var valkeyCmd = &cobra.Command{
	Use:                "valkey [command]",
	Short:              "Control the valkey cache service",
	Long:               `Interact with the Valkey cache service. Supports standard Docker Compose maintenance commands (ps, logs, stop, start, etc.).`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			return cmd.Help()
		}
		if len(args) > 0 && isComposeMaintenanceCommand(args[0]) {
			return proxyServiceToCompose(cmd, "redis", args)
		}
		config := loadConfig()
		if config.Stack.Services.Cache != "valkey" {
			pterm.Warning.Println("Valkey is not enabled in .govard.yml (stack.services.cache=valkey)")
			return nil
		}
		// Strip leading "cli" so `govard valkey cli ping` works like `govard redis cli ping`
		// (runServiceCLI already invokes valkey-cli, so "cli" would become duplicate `valkey-cli cli`).
		if len(args) > 0 && args[0] == "cli" {
			args = args[1:]
		}
		return runServiceCLI("redis", "valkey-cli", args)
	},
}

var elasticsearchCmd = &cobra.Command{
	Use:                "elasticsearch [command|path]",
	Short:              "Control the elasticsearch service",
	Long:               `Interact with the Elasticsearch service. Supports custom queries (via path) and standard Docker Compose maintenance commands (ps, logs, stop, start, etc.).`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			return cmd.Help()
		}
		if len(args) > 0 && isComposeMaintenanceCommand(args[0]) {
			return proxyServiceToCompose(cmd, "elasticsearch", args)
		}
		return runSearchQuery("elasticsearch", 9200, args)
	},
}

var opensearchCmd = &cobra.Command{
	Use:                "opensearch [command|path]",
	Short:              "Control the opensearch service",
	Long:               `Interact with the Opensearch service. Supports custom queries (via path) and standard Docker Compose maintenance commands (ps, logs, stop, start, etc.).`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			return cmd.Help()
		}
		if len(args) > 0 && isComposeMaintenanceCommand(args[0]) {
			return proxyServiceToCompose(cmd, "elasticsearch", args)
		}
		return runSearchQuery("elasticsearch", 9200, args) // We use the service name from the blueprint
	},
}

func runServiceCLI(serviceName string, binary string, args []string) error {
	config := loadConfig()
	containerName := fmt.Sprintf("%s-%s-1", config.ProjectName, serviceName)
	serviceLabel := cases.Title(language.Und).String(serviceName)

	if err := ensureContainerReadyForExec(containerName, serviceLabel); err != nil {
		return err
	}

	pterm.Info.Printf("Connecting to %s on %s...\n", serviceLabel, containerName)

	dockerArgs := dockerExecBaseArgs()
	dockerArgs = append(dockerArgs, containerName, binary)
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if stateErr := ensureContainerReadyForExec(containerName, serviceLabel); stateErr != nil {
			return fmt.Errorf("%s CLI failed: %w", serviceLabel, stateErr)
		}
		return fmt.Errorf("%s CLI failed: %w", serviceLabel, err)
	}
	return nil
}

func runSearchQuery(serviceName string, port int, args []string) error {
	config := loadConfig()
	containerName := fmt.Sprintf("%s-%s-1", config.ProjectName, serviceName)
	serviceLabel := cases.Title(language.Und).String(serviceName)

	if err := ensureContainerReadyForExec(containerName, serviceLabel); err != nil {
		return err
	}

	path := "/"
	if len(args) > 0 {
		path = args[0]
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	pterm.Info.Printf("Querying %s: %s\n", serviceLabel, url)

	dockerArgs := []string{"exec", "-i", containerName, "curl", "-s", "-X", "GET", url}
	c := exec.Command("docker", dockerArgs...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if stateErr := ensureContainerReadyForExec(containerName, serviceLabel); stateErr != nil {
			return fmt.Errorf("%s query failed: %w", serviceLabel, stateErr)
		}
		return fmt.Errorf("%s query failed: %w", serviceLabel, err)
	}
	fmt.Println() // Add newline at the end
	return nil
}
