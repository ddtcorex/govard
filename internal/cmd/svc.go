package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"govard/internal/engine"
	"govard/internal/proxy"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const globalProxyProjectName = "proxy"

var errGlobalServicesNotInitialized = errors.New("global services are not initialized")

var svcCmd = &cobra.Command{
	Use:   "svc",
	Short: "Manage global services and workspace sleep state",
}

var svcUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start global services (proxy, mailpit, pma)",
	Args:  cobra.NoArgs,
	RunE:  runSvcUp,
}

var svcDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop global services (proxy, mailpit, pma)",
	Args:  cobra.NoArgs,
	RunE:  runSvcDown,
}

var svcRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart global services (proxy, mailpit, pma)",
	Args:  cobra.NoArgs,
	RunE:  runSvcRestart,
}

var svcPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest images for global services",
	Args:  cobra.NoArgs,
	RunE:  runSvcPull,
}

var svcPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running global service containers",
	Args:  cobra.NoArgs,
	RunE:  runSvcPs,
}

var svcLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail logs for global services",
	Args:  cobra.NoArgs,
	RunE:  runSvcLogs,
}

var svcSleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Stop all running Govard projects and persist wake state",
	Args:  cobra.NoArgs,
	RunE:  runSvcSleep,
}

var svcWakeCmd = &cobra.Command{
	Use:   "wake",
	Short: "Start all projects recorded in sleep state",
	Args:  cobra.NoArgs,
	RunE:  runSvcWake,
}

var svcLogsTailCount int

func runSvcUp(cmd *cobra.Command, args []string) error {
	pterm.DefaultHeader.Println("Starting Govard Global Services")

	// Check for port conflicts before starting
	if !engine.CheckPortForGovardProxy(cmd.Context(), "80") {
		pterm.Warning.Println("Port 80 is already in use by another process. Govard Proxy might fail to start or route traffic.")
		pterm.Info.Println("Tip: Run `sudo lsof -i :80` to find the conflicting process.")
	}
	if !engine.CheckPortForGovardProxy(cmd.Context(), "443") {
		pterm.Warning.Println("Port 443 is already in use by another process. Govard HTTPS Proxy might fail to start.")
		pterm.Info.Println("Tip: Run `sudo lsof -i :443` to find the conflicting process.")
	}

	if err := engine.EnsureGlobalProxy(); err != nil {
		return fmt.Errorf("ensure global proxy: %w", err)
	}

	pull, _ := cmd.Flags().GetBool("pull")
	if pull {
		pterm.Info.Println("Pulling latest images...")
		if err := runGlobalProxyCompose(cmd, "pull"); err != nil {
			return fmt.Errorf("pull global services: %w", err)
		}
	}

	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	upArgs := []string{"up", "-d"}
	if removeOrphans {
		upArgs = append(upArgs, "--remove-orphans")
	}

	if err := runGlobalProxyCompose(cmd, upArgs...); err != nil {
		return fmt.Errorf("start global services: %w", err)
	}

	if err := registerGlobalServiceRoutes(); err != nil {
		pterm.Warning.Printf("Could not refresh global proxy routes: %v\n", err)
	}

	// Deep revival: re-register routes for all currently running projects
	if err := reviveRunningProjectRoutes(); err != nil {
		pterm.Warning.Printf("Could not fully revive all running project routes: %v\n", err)
	}

	pterm.Success.Println("✅ Global services are running.")
	return nil
}

func reviveRunningProjectRoutes() error {
	running, err := engine.GetRunningProjectNames(context.Background())
	if err != nil {
		return fmt.Errorf("get running projects: %w", err)
	}

	if len(running) == 0 {
		return nil
	}

	pterm.Debug.Printf("Found %d running projects to revive routes for...\n", len(running))

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return fmt.Errorf("read registry: %w", err)
	}

	for _, projectName := range running {
		var matchedEntry *engine.ProjectRegistryEntry
		for _, entry := range entries {
			if entry.ProjectName == projectName {
				matchedEntry = &entry
				break
			}
		}

		if matchedEntry == nil {
			pterm.Debug.Printf("Project %s is running but not found in registry, skipping route revival\n", projectName)
			continue
		}

		// Try to load full config to get the correct proxy target (web vs varnish)
		config, _, err := engine.LoadConfigFromDir(matchedEntry.Path, false)
		if err != nil {
			// Fallback to basic domain from registry if config load fails
			if matchedEntry.Domain != "" {
				target := projectName + "-web-1"
				pterm.Debug.Printf("Reviving basic route for %s -> %s\n", matchedEntry.Domain, target)
				_ = proxy.RegisterDomain(matchedEntry.Domain, target)
			}
			continue
		}

		target := ResolveUpProxyTarget(config)
		for _, domain := range config.AllDomains() {
			pterm.Info.Printf("Reviving route for %s -> %s\n", domain, target)
			if err := proxy.RegisterDomain(domain, target); err != nil {
				pterm.Warning.Printf("Failed to revive route for %s: %v\n", domain, err)
			}
		}
	}

	return nil
}

func runSvcDown(cmd *cobra.Command, args []string) error {
	pterm.DefaultHeader.Println("Stopping Govard Global Services")

	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	downArgs := []string{"down"}
	if removeOrphans {
		downArgs = append(downArgs, "--remove-orphans")
	}

	if err := runGlobalProxyCompose(cmd, downArgs...); err != nil {
		if errors.Is(err, errGlobalServicesNotInitialized) {
			pterm.Warning.Println("Global services are not initialized yet. Run `govard svc up` first.")
			return nil
		}
		return fmt.Errorf("stop global services: %w", err)
	}

	pterm.Success.Println("✅ Global services stopped.")
	return nil
}

func runSvcRestart(cmd *cobra.Command, args []string) error {
	pterm.DefaultHeader.Println("Restarting Govard Global Services")

	if err := runSvcDown(cmd, args); err != nil {
		return err
	}

	return runSvcUp(cmd, args)
}

func runSvcPull(cmd *cobra.Command, args []string) error {
	pterm.DefaultHeader.Println("Pulling Govard Global Services Images")

	if err := runGlobalProxyCompose(cmd, "pull"); err != nil {
		if errors.Is(err, errGlobalServicesNotInitialized) {
			pterm.Warning.Println("Global services are not initialized yet. Run `govard svc up` first.")
			return nil
		}
		return fmt.Errorf("pull global services: %w", err)
	}

	pterm.Success.Println("✅ Global services images pulled.")
	return nil
}

func runSvcPs(cmd *cobra.Command, args []string) error {
	if err := runGlobalProxyCompose(cmd, "ps"); err != nil {
		if errors.Is(err, errGlobalServicesNotInitialized) {
			pterm.Warning.Println("Global services are not initialized yet. Run `govard svc up` first.")
			return nil
		}
		return fmt.Errorf("list global services: %w", err)
	}
	return nil
}

func runSvcLogs(cmd *cobra.Command, args []string) error {
	if err := runGlobalProxyCompose(cmd, "logs", "-f", fmt.Sprintf("--tail=%d", svcLogsTailCount)); err != nil {
		if errors.Is(err, errGlobalServicesNotInitialized) {
			return fmt.Errorf("global services are not initialized yet, run `govard svc up` first")
		}
		return fmt.Errorf("stream global service logs: %w", err)
	}
	return nil
}

func runSvcSleep(cmd *cobra.Command, args []string) error {
	return runSleep()
}

func runSvcWake(cmd *cobra.Command, args []string) error {
	return runWake()
}

func runGlobalProxyCompose(cmd *cobra.Command, args ...string) error {
	composeFile := globalProxyComposeFilePath()
	composeDir := globalProxyComposeDirPath()

	if _, err := os.Stat(composeFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", errGlobalServicesNotInitialized, composeFile)
		}
		return fmt.Errorf("stat global compose file: %w", err)
	}

	dockerArgs := []string{
		"compose",
		"--project-directory",
		composeDir,
		"-p",
		globalProxyProjectName,
		"-f",
		composeFile,
	}
	dockerArgs = append(dockerArgs, args...)

	command := exec.Command("docker", dockerArgs...)
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = cmd.ErrOrStderr()
	return command.Run()
}

func registerGlobalServiceRoutes() error {
	if err := proxy.RegisterDomain("mail.govard.test", "govard-proxy-mail:8025"); err != nil {
		return fmt.Errorf("register mail route: %w", err)
	}
	if err := proxy.RegisterDomain("pma.govard.test", "govard-proxy-pma:80"); err != nil {
		return fmt.Errorf("register pma route: %w", err)
	}
	return nil
}

func globalProxyComposeDirPath() string {
	return filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
}

func globalProxyComposeFilePath() string {
	return filepath.Join(globalProxyComposeDirPath(), "docker-compose.yml")
}

func init() {
	svcUpCmd.Flags().Bool("pull", false, "Pull latest images before starting")
	svcUpCmd.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the compose file")

	svcDownCmd.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the compose file")

	svcLogsCmd.Flags().IntVar(&svcLogsTailCount, "tail", 100, "Number of lines to show from the end of the logs")

	svcCmd.AddCommand(svcUpCmd)
	svcCmd.AddCommand(svcDownCmd)
	svcCmd.AddCommand(svcRestartCmd)
	svcCmd.AddCommand(svcPullCmd)
	svcCmd.AddCommand(svcPsCmd)
	svcCmd.AddCommand(svcLogsCmd)
	svcCmd.AddCommand(svcSleepCmd)
	svcCmd.AddCommand(svcWakeCmd)
}
