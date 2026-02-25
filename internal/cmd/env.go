package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env [command]",
	Short: "Control project environment via docker compose",
	Long: `Manage the lifecycle and services of your project's development environment.
All commands are scoped to the project in the current working directory.
It provides wrappers around common Docker Compose operations and specialized service interactions.

Case Studies:
- Maintenance: Use 'govard env stop' to pause work and 'govard env start' to resume later.
- Troubleshooting: Check 'govard env ps' and 'govard env logs' to identify failing services.
- Cache Management: Run 'govard env redis-cli flushall' to clear local cache.
- Cleanup: Run 'govard env down -v' to completely remove the environment and its data.`,
	Example: `  # Start the project environment
  govard env up

  # List running containers for this project
  govard env ps

  # View real-time logs for all services
  govard env logs

  # Enter a Redis shell for the current project
  govard env redis`,
}

var envUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the project environment",
	Args:  cobra.NoArgs,
	RunE:  runEnvUp,
}

var envStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start existing project containers",
	Args:  cobra.NoArgs,
	RunE:  runEnvStart,
}

var envStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop project containers",
	Args:  cobra.NoArgs,
	RunE:  runEnvStop,
}

var envDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Tear down project containers and networks",
	Args:  cobra.NoArgs,
	RunE:  runEnvDown,
}

var envRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the project environment",
	Args:  cobra.NoArgs,
	RunE:  runEnvRestart,
}

var envPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List project containers",
	Args:  cobra.NoArgs,
	RunE:  runEnvPs,
}

var envLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View project logs",
	Args:  cobra.NoArgs,
	RunE:  runEnvLogs,
}

var envRedisCmd = &cobra.Command{
	Use:   "redis [args]",
	Short: "Interact with project Redis using redis-cli",
	RunE:  runEnvRedis,
}

var envValkeyCmd = &cobra.Command{
	Use:   "valkey [args]",
	Short: "Interact with project Valkey using valkey-cli",
	RunE:  runEnvValkey,
}

var envElasticsearchCmd = &cobra.Command{
	Use:   "elasticsearch [path]",
	Short: "Send a request to project Elasticsearch",
	RunE:  runEnvElasticsearch,
}

var envOpenSearchCmd = &cobra.Command{
	Use:   "opensearch [path]",
	Short: "Send a request to project OpenSearch",
	RunE:  runEnvOpenSearch,
}

var envVarnishCmd = &cobra.Command{
	Use:   "varnish [log|ban|stats]",
	Short: "Project Varnish utility commands",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEnvVarnish,
}

func runEnvUp(cmd *cobra.Command, args []string) error {
	if upCmd.RunE == nil {
		return fmt.Errorf("env up is unavailable")
	}
	return upCmd.RunE(cmd, args)
}

func runEnvStart(cmd *cobra.Command, args []string) error {
	config := loadConfig()
	cwd, _ := os.Getwd()
	composePath := engine.ComposeFilePath(cwd, config.ProjectName)

	command := exec.Command(
		"docker",
		"compose",
		"--project-directory",
		cwd,
		"-p",
		config.ProjectName,
		"-f",
		composePath,
		"start",
	)
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = cmd.ErrOrStderr()
	if err := command.Run(); err != nil {
		return fmt.Errorf("start project containers: %w", err)
	}
	pterm.Success.Println("✅ Environment started.")
	return nil
}

func runEnvStop(cmd *cobra.Command, args []string) error {
	if stopCmd.RunE == nil {
		return fmt.Errorf("env stop is unavailable")
	}
	return stopCmd.RunE(cmd, args)
}

func runEnvDown(cmd *cobra.Command, args []string) error {
	if downCmd.RunE == nil {
		return fmt.Errorf("env down is unavailable")
	}
	return downCmd.RunE(cmd, args)
}

func runEnvRestart(cmd *cobra.Command, args []string) error {
	if err := runEnvStop(cmd, args); err != nil {
		return err
	}
	return runEnvUp(cmd, args)
}

func runEnvPs(cmd *cobra.Command, args []string) error {
	config := loadConfig()
	cwd, _ := os.Getwd()
	composePath := engine.ComposeFilePath(cwd, config.ProjectName)

	command := exec.Command(
		"docker",
		"compose",
		"--project-directory",
		cwd,
		"-p",
		config.ProjectName,
		"-f",
		composePath,
		"ps",
	)
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = cmd.ErrOrStderr()
	if err := command.Run(); err != nil {
		return fmt.Errorf("list project containers: %w", err)
	}
	return nil
}

func runEnvLogs(cmd *cobra.Command, args []string) error {
	if logsCmd.RunE == nil {
		return fmt.Errorf("env logs is unavailable")
	}
	return logsCmd.RunE(cmd, args)
}

func runEnvRedis(cmd *cobra.Command, args []string) error {
	if redisCmd.RunE == nil {
		return fmt.Errorf("env redis is unavailable")
	}
	return redisCmd.RunE(cmd, args)
}

func runEnvValkey(cmd *cobra.Command, args []string) error {
	if valkeyCmd.RunE == nil {
		return fmt.Errorf("env valkey is unavailable")
	}
	return valkeyCmd.RunE(cmd, args)
}

func runEnvElasticsearch(cmd *cobra.Command, args []string) error {
	if elasticsearchCmd.RunE == nil {
		return fmt.Errorf("env elasticsearch is unavailable")
	}
	return elasticsearchCmd.RunE(cmd, args)
}

func runEnvOpenSearch(cmd *cobra.Command, args []string) error {
	if opensearchCmd.RunE == nil {
		return fmt.Errorf("env opensearch is unavailable")
	}
	return opensearchCmd.RunE(cmd, args)
}

func runEnvVarnish(cmd *cobra.Command, args []string) error {
	if varnishCmd.RunE == nil {
		return fmt.Errorf("env varnish is unavailable")
	}
	return varnishCmd.RunE(cmd, args)
}

func init() {
	envUpCmd.Flags().Bool("quickstart", false, "Use a minimal runtime profile for faster first run")
	envDownCmd.Flags().BoolVar(&downRemoveOrphans, "remove-orphans", true, "Remove containers for services not defined in compose file")
	envDownCmd.Flags().BoolVarP(&downVolumes, "volumes", "v", false, "Remove named and anonymous volumes")
	envDownCmd.Flags().StringVar(&downRMI, "rmi", "", "Remove images used by services (all|local)")
	envDownCmd.Flags().IntVarP(&downTimeout, "timeout", "t", 0, "Specify a shutdown timeout in seconds")
	envLogsCmd.Flags().BoolVarP(&errorFilter, "errors", "e", false, "Filter logs for errors only")

	envCmd.AddCommand(envUpCmd)
	envCmd.AddCommand(envStartCmd)
	envCmd.AddCommand(envStopCmd)
	envCmd.AddCommand(envDownCmd)
	envCmd.AddCommand(envRestartCmd)
	envCmd.AddCommand(envPsCmd)
	envCmd.AddCommand(envLogsCmd)
	envCmd.AddCommand(envRedisCmd)
	envCmd.AddCommand(envValkeyCmd)
	envCmd.AddCommand(envElasticsearchCmd)
	envCmd.AddCommand(envOpenSearchCmd)
	envCmd.AddCommand(envVarnishCmd)
}
