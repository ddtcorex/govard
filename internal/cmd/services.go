package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var valkeyCmd = &cobra.Command{
	Use:   "valkey [args]",
	Short: "Interact with the valkey container using valkey-cli",
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadConfig()
		if config.Stack.Services.Cache != "valkey" {
			pterm.Warning.Println("Valkey is not enabled in govard.yml (stack.services.cache=valkey)")
			return nil
		}
		return runServiceCLI("redis", "valkey-cli", args)
	},
}

var elasticsearchCmd = &cobra.Command{
	Use:   "elasticsearch [path]",
	Short: "Send a request to the elasticsearch container",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSearchQuery("elasticsearch", 9200, args)
	},
}

var opensearchCmd = &cobra.Command{
	Use:   "opensearch [path]",
	Short: "Send a request to the opensearch container",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSearchQuery("elasticsearch", 9200, args) // We use the service name from the blueprint
	},
}

func runServiceCLI(serviceName string, binary string, args []string) error {
	config := loadConfig()
	containerName := fmt.Sprintf("%s-%s-1", config.ProjectName, serviceName)

	check := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	if output, err := check.Output(); err != nil || strings.TrimSpace(string(output)) != "true" {
		return fmt.Errorf("%s container %s is not running", strings.Title(serviceName), containerName)
	}

	pterm.Info.Printf("Connecting to %s on %s...\n", strings.Title(serviceName), containerName)

	dockerArgs := []string{"exec", "-it", containerName, binary}
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s CLI failed: %w", strings.Title(serviceName), err)
	}
	return nil
}

func runSearchQuery(serviceName string, port int, args []string) error {
	config := loadConfig()
	containerName := fmt.Sprintf("%s-%s-1", config.ProjectName, serviceName)

	check := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	if output, err := check.Output(); err != nil || strings.TrimSpace(string(output)) != "true" {
		return fmt.Errorf("%s container %s is not running", strings.Title(serviceName), containerName)
	}

	path := "/"
	if len(args) > 0 {
		path = args[0]
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	pterm.Info.Printf("Querying %s: %s\n", strings.Title(serviceName), url)

	c := exec.Command("docker", "exec", "-i", containerName, "curl", "-s", "-X", "GET", url)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s query failed: %w", strings.Title(serviceName), err)
	}
	fmt.Println() // Add newline at the end
	return nil
}
