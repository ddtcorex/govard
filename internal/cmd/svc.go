package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/proxy"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

const globalProxyProjectName = "proxy"

var errGlobalServicesNotInitialized = errors.New("global services are not initialized")

var svcCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "svc",
	Short: "Manage global services and workspace sleep state",
	Long: strings.TrimSpace(`
Manage global shared services (Proxy, Mailpit, PHPMyAdmin, Portainer) and control the workspace state.
Global services are shared across all projects.

Govard intelligently proxies global Docker Compose commands to the shared service stack.
Govard-specific toggles on 'up'/'restart' include:
- --pull: pull latest images before startup
- --no-trust: skip Govard Root CA trust installation
- --no-fallback: disable automatic local image build retry if pulls fail

Case Studies:
- Setup: Use 'govard svc up' to start the global proxy and shared utilities.
- Troubleshooting: Use 'govard svc logs' or 'govard svc ps' to check global service health.
- Optimization: Use 'govard svc sleep' to pause all running project containers at once.
`),
	Example: `  # Start global services (Proxy, Mail, etc.)
  govard svc up

  # Start global services without CA trust installation
  govard svc up --no-trust

  # Disable automatic local image build fallback
  govard svc up --no-fallback

  # Stop all global services
  govard svc down

  # Pause all active project environments
  govard svc sleep

  # View help for all supported global compose commands
  govard svc --help`,
	Args: cobra.ArbitraryArgs,
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			proxyArgs := []string{}
			for i, arg := range os.Args {
				if arg == "svc" && i+1 < len(os.Args) {
					proxyArgs = os.Args[i+1:]
					break
				}
			}
			if len(proxyArgs) > 0 {
				return runGlobalProxyCompose(cmd, proxyArgs...)
			}
		}
		return cmd.Help()
	},
}

var svcSleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Stop all running Govard projects and persist wake state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSleep()
	},
}

var svcWakeCmd = &cobra.Command{
	Use:   "wake",
	Short: "Start all projects recorded in sleep state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWake()
	},
}

func runGlobalProxyCompose(cmd *cobra.Command, args ...string) error {
	subcommand := args[0]
	composeFile := globalProxyComposeFilePath()
	composeDir := globalProxyComposeDirPath()

	if _, err := os.Stat(composeFile); err != nil {
		if errors.Is(err, os.ErrNotExist) && subcommand == "up" {
			if err := engine.EnsureGlobalProxy(); err != nil {
				return fmt.Errorf("ensure global proxy: %w", err)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", errGlobalServicesNotInitialized, composeFile)
		} else {
			return fmt.Errorf("stat global compose file: %w", err)
		}
	}

	switch subcommand {
	case "up":
		return handleSvcUp(cmd, args)
	case "down":
		return handleSvcDown(cmd, args)
	case "restart":
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Restarting Govard Global Services ")
		fmt.Println()
		_ = handleSvcDown(cmd, []string{"down"})
		return handleSvcUp(cmd, append([]string{"up"}, args[1:]...))
	}

	return engine.RunCompose(cmd.Context(), engine.ComposeOptions{
		ProjectDir:  composeDir,
		ProjectName: globalProxyProjectName,
		ComposeFile: composeFile,
		Args:        args,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	})
}

func handleSvcUp(cmd *cobra.Command, args []string) error {
	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Starting Govard Global Services ")
	fmt.Println()

	ctx := cmd.Context()
	if !engine.CheckPortForGovardProxy(ctx, "80") {
		pterm.Warning.Println("Port 80 is in use. Govard Proxy might fail.")
	}
	if !engine.CheckPortForGovardProxy(ctx, "443") {
		pterm.Warning.Println("Port 443 is in use. Govard HTTPS Proxy might fail.")
	}

	// Always ensure proxy files are there
	if err := engine.EnsureGlobalProxy(); err != nil {
		return err
	}

	// Extract Govard-specific flags from os.Args if they exist
	pull := hasFlag("--pull")
	fallback := !hasFlag("--no-fallback")

	composeFile := globalProxyComposeFilePath()
	composeDir := globalProxyComposeDirPath()

	if pull {
		pterm.Info.Println("Pulling latest images...")
		_ = engine.RunCompose(ctx, engine.ComposeOptions{
			ProjectDir: composeDir, ProjectName: globalProxyProjectName, ComposeFile: composeFile,
			Args: []string{"pull"}, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
		})
	}

	// Prepare standard 'up' args
	upArgs := []string{"up", "-d"}
	for _, arg := range args[1:] {
		if arg != "-d" && arg != "--detach" {
			upArgs = append(upArgs, arg)
		}
	}

	err := engine.RunCompose(ctx, engine.ComposeOptions{
		ProjectDir: composeDir, ProjectName: globalProxyProjectName, ComposeFile: composeFile,
		Args: upArgs, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), Stdin: os.Stdin,
	})

	if err != nil && fallback {
		pterm.Warning.Printf("Start failed: %v. Attempting local build fallback...\n", err)
		built, _ := engine.FallbackBuildMissingGovardImagesFromCompose(composeFile, cmd.OutOrStdout(), cmd.ErrOrStderr())
		if len(built) > 0 {
			pterm.Info.Println("Retrying start...")
			err = engine.RunCompose(ctx, engine.ComposeOptions{
				ProjectDir: composeDir, ProjectName: globalProxyProjectName, ComposeFile: composeFile,
				Args: upArgs, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			})
		}
	}

	if err != nil {
		return err
	}

	if waitForGlobalProxyReady(ctx, 8*time.Second) {
		_ = registerGlobalServiceRoutes()
		_ = reviveRunningProjectRoutes()
		if !hasFlag("--no-trust") {
			_ = engine.TrustCAWithOptions(engine.TrustOptions{ImportBrowsers: true, ContinueOnBrowserError: true})
		}
		pterm.Success.Println("✅ Global services are running.")
	} else {
		return fmt.Errorf("global proxy not ready")
	}
	return nil
}

func handleSvcDown(cmd *cobra.Command, args []string) error {
	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Stopping Govard Global Services ")
	fmt.Println()
	err := engine.RunCompose(cmd.Context(), engine.ComposeOptions{
		ProjectDir:  globalProxyComposeDirPath(),
		ProjectName: globalProxyProjectName,
		ComposeFile: globalProxyComposeFilePath(),
		Args:        args,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
	})
	if err == nil {
		pterm.Success.Println("✅ Global services stopped.")
	}
	return err
}

func reviveRunningProjectRoutes() error {
	running, err := engine.GetRunningProjectNames(context.Background())
	if err != nil || len(running) == 0 {
		return err
	}

	entries, _ := engine.ReadProjectRegistryEntries()
	for _, projectName := range running {
		for _, entry := range entries {
			if entry.ProjectName == projectName {
				config, _, err := engine.LoadConfigFromDir(entry.Path, false)
				if err == nil {
					target := ResolveUpProxyTarget(config)
					allDomains := config.AllDomains()
					if len(allDomains) > 0 {
						_ = proxy.RegisterDomains(allDomains, target)
					}
				}
				break
			}
		}
	}
	return nil
}

func waitForGlobalProxyReady(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if engine.IsContainerRunning(ctx, "govard-proxy-caddy") || engine.IsContainerRunning(ctx, "proxy-caddy-1") {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
	return false
}

func registerGlobalServiceRoutes() error {
	_ = proxy.RegisterDomain("mail.govard.test", "govard-proxy-mail:8025")
	_ = proxy.RegisterDomain("pma.govard.test", "govard-proxy-pma:80")
	_ = proxy.RegisterDomain("portainer.govard.test", "govard-proxy-portainer:9000")
	return nil
}

func globalProxyComposeDirPath() string {
	return filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
}

func globalProxyComposeFilePath() string {
	return filepath.Join(globalProxyComposeDirPath(), "docker-compose.yml")
}

func hasFlag(name string) bool {
	for _, arg := range os.Args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func init() {
	svcCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		rebrandComposeHelp(cmd, "svc")
	})

	svcCmd.AddCommand(svcSleepCmd)
	svcCmd.AddCommand(svcWakeCmd)
}
