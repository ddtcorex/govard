package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var rabbitmqCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "rabbitmq [command]",
	Short: "Control the rabbitmq queue service",
	Long: `Interact with the RabbitMQ queue service.
Supports custom utility commands (status, queues, cli) and standard Docker Compose maintenance commands (ps, logs, stop, start, etc.).`,
	Example: `  # Check RabbitMQ node status
  govard env rabbitmq status

  # List queues with message/consumer counts
  govard env rabbitmq queues

  # Run an arbitrary rabbitmqctl command
  govard env rabbitmq cli list_exchanges

  # Check RabbitMQ container status
  govard env rabbitmq ps`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
				return cmd.Help()
			}
			if isComposeMaintenanceCommand(args[0]) {
				return proxyServiceToCompose(cmd, "rabbitmq", args)
			}
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		config := loadConfig()
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.RabbitMQSuffix)

		switch args[0] {
		case "status":
			return runRabbitMQCtl(containerName, []string{"status"})
		case "queues":
			return runRabbitMQCtl(containerName, []string{"list_queues", "name", "messages", "consumers"})
		case "cli":
			return runRabbitMQCtl(containerName, args[1:])
		default:
			return fmt.Errorf("unknown rabbitmq subcommand: %s", args[0])
		}
	},
}

func runRabbitMQCtl(containerName string, args []string) error {
	if err := ensureContainerReadyForExec(containerName, "RabbitMQ"); err != nil {
		return err
	}

	dockerArgs := dockerExecBaseArgs()
	dockerArgs = append(dockerArgs, containerName, "rabbitmqctl")
	dockerArgs = append(dockerArgs, args...)

	c := exec.Command("docker", dockerArgs...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if stateErr := ensureContainerReadyForExec(containerName, "RabbitMQ"); stateErr != nil {
			return fmt.Errorf("rabbitmq command failed: %w", stateErr)
		}
		return fmt.Errorf("rabbitmq command failed: %w", err)
	}
	return nil
}
