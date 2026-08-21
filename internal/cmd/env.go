package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/proxy"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type EnvDependenciesForTest struct {
	RunCompose                func(context.Context, engine.ComposeOptions) error
	RegisterDomains           func([]string, string) error
	UnregisterDomain          func(string) error
	RegisterSearchDomains     func([]string, string) error
	UnregisterSearchDomain    func(string) error
	AddHostsEntry             func(string) error
	RemoveHostsEntry          func(string) error
	IsDomainResolvableLocally func(string) bool
	RunHooks                  func(engine.Config, string, io.Writer, io.Writer) error
	RefreshPMAActiveProjects  func() error
}

var envDeps = EnvDependenciesForTest{
	RunCompose:                engine.RunCompose,
	RegisterDomains:           proxy.RegisterDomains,
	UnregisterDomain:          proxy.UnregisterDomain,
	RegisterSearchDomains:     proxy.RegisterSearchDomains,
	UnregisterSearchDomain:    proxy.UnregisterSearchDomain,
	AddHostsEntry:             engine.AddHostsEntry,
	RemoveHostsEntry:          engine.RemoveHostsEntry,
	IsDomainResolvableLocally: engine.IsDomainResolvableLocally,
	RunHooks:                  engine.RunHooks,
	RefreshPMAActiveProjects:  engine.RefreshPMAActiveProjects,
}

var envCmd = &cobra.Command{
	Use:   "env [command]",
	Short: "Control project environment via docker compose",
	Long: `Manage the lifecycle and services of your project's development environment.
All commands are scoped to the project in the current working directory.
It provides smart wrappers around Docker Compose operations and specialized service interactions.

Govard intelligently proxies almost all Docker Compose commands. If a command is not
explicitly handled by Govard, it is passed through to 'docker compose' with the 
correct project context.

Aliases: project

Case Studies:
- Maintenance: Use 'govard env stop' to pause work and 'govard env start' to resume later.
- Troubleshooting: Check 'govard env ps' and 'govard env logs' to identify failing services.
- Cache Management: Run 'govard redis flush' to clear local cache.
- Cleanup: Run 'govard env down -v' to completely remove the environment and its data.`,
	Example: `  # Start the project environment
  govard env up

  # View help for all supported compose commands
  govard env --help

  # List running containers for this project
  govard env ps

  # View real-time logs for all services
  govard env logs -f

  # Enter a Redis shell for the current project
  govard redis cli`,
	Args:    cobra.ArbitraryArgs,
	Aliases: []string{},
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			proxyArgs := []string{}
			for i, arg := range os.Args {
				if (arg == "env" || arg == "project") && i+1 < len(os.Args) {
					proxyArgs = os.Args[i+1:]
					break
				}
			}
			if len(proxyArgs) > 0 {
				return proxyEnvToCompose(cmd, proxyArgs)
			}
		}
		return cmd.Help()
	},
}

func proxyEnvToCompose(cmd *cobra.Command, args []string) error {
	subcommand := args[0]

	// Read --profile from parent command if present
	profile, _ := cmd.Flags().GetString("profile")

	config := loadConfigWithProfile(profile)
	cwd, _ := os.Getwd()
	composePath := engine.ComposeFilePathWithProfile(cwd, config.ProjectName, config.Profile)

	switch subcommand {
	case "start":
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Starting Govard Project ")
		fmt.Println()
		if config.Profile != "" {
			pterm.Info.Printf("Using profile: %s\n", config.Profile)
		}
		if err := startProjectEnvironment(cmd, config, cwd, composePath, args[1:]); err != nil {
			return err
		}
		pterm.Success.Println("✅ Environment started.")
		return nil
	case "restart":
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Restarting Govard Project ")
		fmt.Println()
		if config.Profile != "" {
			pterm.Info.Printf("Using profile: %s\n", config.Profile)
		}
		if err := proxyEnvToCompose(cmd, []string{"stop"}); err != nil {
			return err
		}
		if err := startProjectEnvironment(cmd, config, cwd, composePath, args[1:]); err != nil {
			return err
		}
		pterm.Success.Println("✅ Environment restarted.")
		return nil
	case "stop", "down":
		action := "Stopping"
		if subcommand == "down" {
			action = "Tearing Down"
		}
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Printf(" %s Govard Environment \n", action)
		fmt.Println()

		if err := envDeps.RunHooks(config, engine.HookPreStop, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-stop hooks failed: %w", err)
		}

		err := envDeps.RunCompose(cmd.Context(), engine.ComposeOptions{
			ProjectDir:  cwd,
			ProjectName: config.ProjectName,
			ComposeFile: composePath,
			Args:        args,
			Stdout:      cmd.OutOrStdout(),
			Stderr:      cmd.ErrOrStderr(),
			Stdin:       os.Stdin,
		})
		if err != nil {
			return fmt.Errorf("failed to %s containers: %w", strings.ToLower(subcommand), err)
		}

		for _, domain := range config.AllDomains() {
			if err := envDeps.UnregisterDomain(domain); err != nil {
				pterm.Warning.Printf("Could not remove proxy route for %s: %v\n", domain, err)
			}
			// Unregister unconditionally (not gated on Stack.Features.Search): if search
			// was previously enabled and later disabled, a stale srv_search route could
			// otherwise linger. UnregisterSearchDomain is a safe no-op when no matching
			// route exists.
			if err := envDeps.UnregisterSearchDomain(domain); err != nil {
				pterm.Warning.Printf("Could not remove search proxy route for %s: %v\n", domain, err)
			}
			if err := envDeps.RemoveHostsEntry(domain); err != nil {
				pterm.Warning.Printf("Could not remove hosts entry for %s: %v\n", domain, err)
			}
		}

		if err := envDeps.RunHooks(config, engine.HookPostStop, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("post-stop hooks failed: %w", err)
		}

		pterm.Success.Printf("✅ Environment %s.\n", strings.ToLower(action))
		return nil

	case "logs":
		// Check for --errors flag
		hasErrorFilter := false
		for _, arg := range args {
			if arg == "--errors" {
				hasErrorFilter = true
				break
			}
		}

		if hasErrorFilter {
			fmt.Println()
			pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Log Stream (Errors Only) ")
			fmt.Println()
			pterm.Info.Println("Filtering for errors...")

			// Rebuild args without --errors
			filteredArgs := []string{}
			for _, arg := range args {
				if arg != "--errors" {
					filteredArgs = append(filteredArgs, arg)
				}
			}

			// Construct manual grep command
			composeCmd := fmt.Sprintf("docker compose --project-directory %s -p %s -f %s",
				engine.ShellQuote(cwd), engine.ShellQuote(config.ProjectName), engine.ShellQuote(composePath))

			logArgs := strings.Join(filteredArgs, " ")
			if !strings.Contains(logArgs, "-f") && !strings.Contains(logArgs, "--follow") {
				logArgs += " -f --tail=100"
			}

			filterCommand := fmt.Sprintf("%s %s | grep -iE 'error|critical|fail|exception'", composeCmd, logArgs)
			c := exec.Command("sh", "-c", filterCommand)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			return c.Run()
		}

	case "pull":
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Pulling Project Images ")
		fmt.Println()
		fallbackLocal := !hasFlag("--no-fallback")
		extraPullArgs := make([]string, 0, len(args))
		for _, arg := range args[1:] {
			if arg == "--no-fallback" || strings.HasPrefix(arg, "--no-fallback=") {
				continue
			}
			extraPullArgs = append(extraPullArgs, arg)
		}
		pullArgs := append([]string{"pull", "--ignore-buildable"}, extraPullArgs...)

		err := engine.RunCompose(cmd.Context(), engine.ComposeOptions{
			ProjectDir:  cwd,
			ProjectName: config.ProjectName,
			ComposeFile: composePath,
			Args:        pullArgs,
			Stdout:      cmd.OutOrStdout(),
			Stderr:      cmd.ErrOrStderr(),
			Stdin:       os.Stdin,
		})
		if err != nil {
			// A single broken image must not stop the remaining pulls:
			// retry image by image so only failing ones fall back or fail.
			pterm.Warning.Printf("docker compose pull failed: %v\n", err)
			pterm.Info.Println("Retrieving project images one by one...")

			result, resilientErr := engine.RunResilientImagePullsFromCompose(cmd.Context(), composePath, engine.ResilientPullOptions{
				FallbackLocalBuild: fallbackLocal,
			}, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if resilientErr != nil {
				return fmt.Errorf("pull project images: %w", resilientErr)
			}

			if len(result.Pulled) > 0 {
				pterm.Success.Printf("Pulled %d image(s): %s\n", len(result.Pulled), strings.Join(result.Pulled, ", "))
			}
			if len(result.ReusedLocal) > 0 {
				pterm.Success.Printf("Kept %d existing local image(s): %s\n", len(result.ReusedLocal), strings.Join(result.ReusedLocal, ", "))
			}
			if len(result.BuiltLocally) > 0 {
				pterm.Success.Printf("Local fallback built %d image(s): %s\n", len(result.BuiltLocally), strings.Join(result.BuiltLocally, ", "))
			}
			for _, failure := range result.Failed {
				pterm.Warning.Printf("Could not prepare %s: %v\n", failure.Image, failure.Err)
			}
			if len(result.Failed) == 0 {
				pterm.Info.Println("All project images are available.")
			}
		}
		built, rebuildErr := engine.RebuildIncompatibleGovardImagesFromCompose(composePath, cmd.OutOrStdout(), cmd.ErrOrStderr())
		if rebuildErr != nil {
			return fmt.Errorf("rebuild incompatible local Govard images: %w", rebuildErr)
		}
		if len(built) > 0 {
			pterm.Success.Printf("Rebuilt %d incompatible local image(s): %v\n", len(built), built)
		}
		pterm.Success.Println("✅ Images pulled.")
		return nil
	case "cleanup":
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Cleaning Up Compose Cache ")
		fmt.Println()
		count, err := engine.CleanupStaleComposeFiles(7 * 24 * time.Hour)
		if err != nil {
			return fmt.Errorf("cleanup compose cache: %w", err)
		}
		pterm.Success.Printf("✅ Removed %d stale compose file(s).\n", count)
		return nil
	}

	// Default proxy for everything else
	return envDeps.RunCompose(cmd.Context(), engine.ComposeOptions{
		ProjectDir:  cwd,
		ProjectName: config.ProjectName,
		ComposeFile: composePath,
		Args:        args,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	})
}

func startProjectEnvironment(cmd *cobra.Command, config engine.Config, cwd string, composePath string, extraArgs []string) error {
	args := append([]string{"up", "-d"}, extraArgs...)
	if err := envDeps.RunCompose(cmd.Context(), engine.ComposeOptions{
		ProjectDir:  cwd,
		ProjectName: config.ProjectName,
		ComposeFile: composePath,
		Args:        args,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	}); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	ensureProjectDomainsAvailable(config)
	return nil
}

func ensureProjectDomainsAvailable(config engine.Config) {
	target := ResolveUpProxyTarget(config)
	allDomains := config.AllDomains()

	var proxyErr error
	if len(allDomains) > 0 {
		proxyErr = envDeps.RegisterDomains(allDomains, target)
	}

	if config.Stack.Features.Search && len(allDomains) > 0 {
		searchTarget := ResolveUpSearchProxyTarget(config)
		if searchErr := envDeps.RegisterSearchDomains(allDomains, searchTarget); searchErr != nil {
			pterm.Warning.Printf("Could not register search proxy route: %v\n", searchErr)
		}
	}

	for _, domain := range allDomains {
		if envDeps.IsDomainResolvableLocally(domain) {
			pterm.Success.Printf("Domain %s already resolves locally\n", domain)
		} else if err := envDeps.AddHostsEntry(domain); err != nil {
			pterm.Warning.Printf("Could not update hosts file for %s: %v\n", domain, err)
			pterm.Info.Printf("Please manually add '127.0.0.1 %s' to your hosts file.\n", domain)
		} else {
			pterm.Success.Printf("Domain %s mapped to 127.0.0.1\n", domain)
		}

		if proxyErr != nil {
			pterm.Warning.Printf("Could not register domain %s with Govard Proxy: %v\n", domain, proxyErr)
		} else {
			pterm.Success.Printf("Domain %s registered with Govard Proxy -> %s\n", domain, target)
		}
	}

	if err := envDeps.RefreshPMAActiveProjects(); err != nil {
		pterm.Warning.Printf("Could not refresh PMA active projects: %v\n", err)
	}
}

func SetEnvDependenciesForTest(deps EnvDependenciesForTest) func() {
	previous := envDeps

	if deps.RunCompose != nil {
		envDeps.RunCompose = deps.RunCompose
	} else {
		envDeps.RunCompose = engine.RunCompose
	}
	if deps.RegisterDomains != nil {
		envDeps.RegisterDomains = deps.RegisterDomains
	} else {
		envDeps.RegisterDomains = proxy.RegisterDomains
	}
	if deps.UnregisterDomain != nil {
		envDeps.UnregisterDomain = deps.UnregisterDomain
	} else {
		envDeps.UnregisterDomain = proxy.UnregisterDomain
	}
	if deps.RegisterSearchDomains != nil {
		envDeps.RegisterSearchDomains = deps.RegisterSearchDomains
	} else {
		envDeps.RegisterSearchDomains = proxy.RegisterSearchDomains
	}
	if deps.UnregisterSearchDomain != nil {
		envDeps.UnregisterSearchDomain = deps.UnregisterSearchDomain
	} else {
		envDeps.UnregisterSearchDomain = proxy.UnregisterSearchDomain
	}
	if deps.AddHostsEntry != nil {
		envDeps.AddHostsEntry = deps.AddHostsEntry
	} else {
		envDeps.AddHostsEntry = engine.AddHostsEntry
	}
	if deps.RemoveHostsEntry != nil {
		envDeps.RemoveHostsEntry = deps.RemoveHostsEntry
	} else {
		envDeps.RemoveHostsEntry = engine.RemoveHostsEntry
	}
	if deps.IsDomainResolvableLocally != nil {
		envDeps.IsDomainResolvableLocally = deps.IsDomainResolvableLocally
	} else {
		envDeps.IsDomainResolvableLocally = engine.IsDomainResolvableLocally
	}
	if deps.RunHooks != nil {
		envDeps.RunHooks = deps.RunHooks
	} else {
		envDeps.RunHooks = engine.RunHooks
	}
	if deps.RefreshPMAActiveProjects != nil {
		envDeps.RefreshPMAActiveProjects = deps.RefreshPMAActiveProjects
	} else {
		envDeps.RefreshPMAActiveProjects = engine.RefreshPMAActiveProjects
	}

	return func() {
		envDeps = previous
	}
}

func ProxyEnvToComposeForTest(cmd *cobra.Command, args []string) error {
	return proxyEnvToCompose(cmd, args)
}

func init() {
	envCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		rebrandComposeHelp(cmd, "env")
	})

	// Non-standard shortcuts
	envCmd.AddCommand(upCmd)
	envCmd.AddCommand(redisCmd)
	envCmd.AddCommand(valkeyCmd)
	envCmd.AddCommand(elasticsearchCmd)
	envCmd.AddCommand(opensearchCmd)
	envCmd.AddCommand(varnishCmd)
	envCmd.AddCommand(rabbitmqCmd)

	envCmd.AddCommand(envCleanupCmd)
}

var envCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Prune stale docker-compose files and manifests from Govard home",
	RunE: func(cmd *cobra.Command, args []string) error {
		return proxyEnvToCompose(cmd, []string{"cleanup"})
	},
}
