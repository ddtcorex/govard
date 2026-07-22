package cmd

import (
	"context"
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/proxy"
	"govard/internal/updater"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up [flags]",
	Short: "Start the development environment",
	Long: `Starts all Docker containers required for the current project.
It automatically handles framework detection, configuration validation,
Docker Compose blueprint rendering, host mapping, and proxy registration.

The startup process follows these stages:
1. Detect: Identifies framework context from project files.
2. Validate: Checks Docker status, port conflicts, and layered config.
3. Render: Generates the specialized Docker Compose file.
4. Start: Runs 'docker compose up' in detached mode.
5. Ready: Waits for the local runtime to accept requests.
6. Verify: Maps the .test domain to 127.0.0.1 and registers it with the Govard Proxy.

Case Studies:
- Standard Startup: Simply run 'govard env up' to get your full stack running.
- Low Resource Mode: Use --quickstart if you have limited RAM or only need PHP/Web server.
- Fresh Install Recovery: If containers are broken, 'govard env up' re-renders and restarts them.
- Skip Auto-Configuration: Use --no-tuning to skip framework auto-configuration (Magento, etc.)`,
	Example: `  # Start the environment normally
  govard env up

  # Fast startup: skip heavy services like Elasticsearch, Varnish, Redis
  govard env up --quickstart

  # Start without framework auto-configuration
  govard env up --no-tuning`,
	RunE: runUpCommand,
}

const defaultUpReadinessTimeout = 90 * time.Second

var upReadinessProbeInterval = 1500 * time.Millisecond
var upReadinessSleep = time.Sleep
var upReadinessProbeRunner = func(containerName string, probeArgs []string) error {
	args := append([]string{"exec", containerName}, probeArgs...)
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%w (%s)", err, trimmed)
}
var upContainerStateRunner = func(containerName string) (string, error) {
	cmd := exec.Command(
		"docker",
		"inspect",
		"-f",
		"{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.State.Error}}",
		containerName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%w (%s)", err, trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

var upRefreshRunningProjectNames = engine.GetRunningProjectNames
var upRefreshReadProjectRegistryEntries = engine.ReadProjectRegistryEntries
var upRefreshLoadConfigFromDir = engine.LoadConfigFromDir
var upRefreshRenderBlueprint = engine.RenderBlueprint
var upRefreshRunCompose = engine.RunCompose

type upPipelineStage struct {
	Name         string
	OnFailureTip string
	Run          func() error
}

type upReadinessCheck struct {
	Service       string
	ContainerName string
	ProbeArgs     []string
}

type upContainerState struct {
	Status    string
	ExitCode  string
	OOMKilled string
	Error     string
}

type upRuntimeContext struct {
	Cwd           string
	Config        engine.Config
	Compose       string
	Loaded        []string
	Metadata      engine.ProjectMetadata
	Profile       string
	Quickstart    bool
	Pull          bool
	FallbackLocal bool
	RemoveOrphans bool
	ForceRecreate bool
	UpdateLock    bool
	SkipTuning    bool
	ShiftInfo     *engine.ProfileShiftInfo
	Out           io.Writer
	Err           io.Writer
}

func buildUpPipelineStages(cmd *cobra.Command, context *upRuntimeContext) []upPipelineStage {
	return []upPipelineStage{
		{
			Name:         "Detect",
			OnFailureTip: "govard init",
			Run: func() error {
				context.Metadata = engine.DetectFramework(context.Cwd)
				if strings.TrimSpace(context.Metadata.Version) == "" {
					pterm.Info.Printf("Detected framework: %s\n", context.Metadata.Framework)
				} else {
					pterm.Info.Printf("Detected framework: %s (%s)\n", context.Metadata.Framework, context.Metadata.Version)
				}
				return nil
			},
		},
		{
			Name:         "Validate",
			OnFailureTip: "govard doctor",
			Run: func() error {
				var err error
				context.Config, context.Loaded, err = engine.LoadConfigFromDirWithProfile(context.Cwd, true, context.Profile)
				if err != nil {
					return fmt.Errorf("load layered config: %w", err)
				}
				rawConfig, _ := engine.LoadRawConfigFromDirWithProfile(context.Cwd, false, context.Profile)
				if warnings := engine.CollectProfileSyncWarnings(rawConfig, context.Metadata); len(warnings) > 0 {
					for _, warning := range warnings {
						pterm.Warning.Println(warning)
					}
				}
				if context.Quickstart {
					ApplyQuickstartProfile(&context.Config)
					pterm.Info.Println("Quickstart profile enabled: optional services reduced for faster first run.")
				}
				if err := engine.ValidateProjectIdentityUniqueness(context.Cwd, context.Config); err != nil {
					return err
				}
				context.Compose = engine.ComposeFilePathWithProfile(context.Cwd, context.Config.ProjectName, context.Profile)
				pterm.Info.Printf("Loaded config layers: %d\n", len(context.Loaded))
				if context.Profile != "" {
					pterm.Info.Printf("Using profile: %s\n", context.Profile)
				}
				lockWarnings, lockErr := evaluateUpLockPolicy(context.Cwd, context.Config, context.UpdateLock)
				for _, warning := range lockWarnings {
					pterm.Warning.Println(warning)
				}
				if len(lockWarnings) > 0 {
					pterm.Warning.Println("Run `govard lock check` for full lockfile compliance details.")
				}
				if lockErr != nil {
					return lockErr
				}

				if err := engine.CheckDockerStatus(cmd.Context()); err != nil {
					return fmt.Errorf("docker daemon is not ready: %w", err)
				}
				if err := engine.CheckDockerComposePlugin(cmd.Context()); err != nil {
					return err
				}

				if !engine.CheckPortForGovardProxy(cmd.Context(), "80") {
					pterm.Warning.Println("Port 80 is currently in use. Proxy routing may fail.")
				}
				if !engine.CheckPortForGovardProxy(cmd.Context(), "443") {
					pterm.Warning.Println("Port 443 is currently in use. HTTPS proxy routing may fail.")
				}
				if err := engine.CheckDiskScratchWrite(); err != nil {
					return fmt.Errorf("disk scratch check failed: %w", err)
				}
				if err := engine.CheckNetworkConnectivity(); err != nil {
					pterm.Warning.Printf("Network outbound probe failed: %v\n", err)
				}
				return nil
			},
		},
		{
			Name:         "ProfileGuard",
			OnFailureTip: "govard config profile apply",
			Run: func() error {
				shift := engine.DetectProfileShift(context.Config)
				if !shift.Shifted {
					return nil
				}

				// Clear previous_profile from registry after detection
				// This ensures shift is only processed once
				if err := engine.ClearPreviousProfile(context.Cwd); err != nil {
					pterm.Warning.Printf("Could not clear previous profile: %v\n", err)
				}

				// Prompt user for confirmation when the shift is implicit
				// (no --profile flag, not initial config, and TTY is available)
				explicitProfile := context.Profile != ""
				if !shift.IsInitial && !explicitProfile && stdinIsTerminal() {
					pterm.Warning.Printf("Profile shift detected: %s\n", shift.Reason)
					if shift.PreviousVersion != "" && shift.CurrentVersion != "" {
						pterm.Info.Printf("  Previous: %s (PHP %s)\n", shift.PreviousVersion, shift.PreviousPHP)
						pterm.Info.Printf("  Current:  %s (PHP %s)\n", shift.CurrentVersion, shift.CurrentPHP)
					}
					pterm.Info.Println("This will recreate infrastructure containers (Redis, etc.) to match the new profile.")

					proceed, promptErr := pterm.DefaultInteractiveConfirm.
						WithDefaultValue(true).
						Show("Continue with profile switch?")
					if promptErr != nil || !proceed {
						return fmt.Errorf("aborted by user")
					}
				} else if !shift.IsInitial {
					pterm.Info.Printf("Profile shift: %s. Preparing infrastructure...\n", shift.Reason)
				}

				// Pre-clean infrastructure before containers start
				engine.PrepareInfraForShift(context.Config.ProjectName, context.Config)
				context.ForceRecreate = true
				context.ShiftInfo = &shift
				return nil
			},
		},
		{
			Name:         "Render",
			OnFailureTip: "govard doctor --fix",
			Run: func() error {
				if err := engine.RunHooks(context.Config, engine.HookPreUp, context.Out, context.Err); err != nil {
					return fmt.Errorf("pre-up hooks failed: %w", err)
				}

				if err := engine.EnsureGlobalProxy(); err != nil {
					pterm.Warning.Printf("Govard Proxy not ready: %v\n", err)
				}

				if err := engine.RenderBlueprintWithProfile(context.Cwd, context.Config, context.Profile); err != nil {
					return fmt.Errorf("render blueprint: %w", err)
				}
				pterm.Success.Printf("Rendered compose file: %s\n", context.Compose)
				return nil
			},
		},
		{
			Name:         "SyncResources",
			OnFailureTip: "govard db clone-volume",
			Run: func() error {
				return ensureUpProfileResourceSync(context)
			},
		},
		{
			Name:         "Pull",
			OnFailureTip: "govard doctor --fix",
			Run: func() error {
				if !context.Pull {
					return nil
				}
				err := engine.RunCompose(cmd.Context(), engine.ComposeOptions{
					ProjectDir:  context.Cwd,
					ProjectName: context.Config.ProjectName,
					ComposeFile: context.Compose,
					Args:        []string{"pull"},
					Stdout:      context.Out,
					Stderr:      context.Err,
				})
				if err != nil {
					if !context.FallbackLocal {
						return fmt.Errorf("docker compose pull failed: %w", err)
					}

					pterm.Warning.Printf("docker compose pull failed: %v\n", err)
					pterm.Info.Println("Attempting local Govard image build fallback...")

					built, fallbackErr := engine.FallbackBuildMissingGovardImagesFromCompose(context.Compose, context.Out, context.Err)
					if fallbackErr != nil {
						return fmt.Errorf("docker compose pull failed: %w (local fallback failed: %v)", err, fallbackErr)
					}
					if len(built) == 0 {
						pterm.Warning.Println("No missing Govard-managed images required local build. Continuing with current local cache.")
						return nil
					}

					pterm.Success.Printf("Local fallback built %d image(s): %s\n", len(built), strings.Join(built, ", "))
				}
				return nil
			},
		},
		{
			Name:         "LocalImages",
			OnFailureTip: "govard doctor --fix",
			Run: func() error {
				if !context.FallbackLocal {
					return nil
				}
				built, err := engine.RebuildIncompatibleGovardImagesFromCompose(context.Compose, context.Out, context.Err)
				if err != nil {
					return fmt.Errorf("rebuild incompatible local Govard images: %w", err)
				}
				if len(built) > 0 {
					pterm.Success.Printf("Rebuilt %d incompatible local image(s): %s\n", len(built), strings.Join(built, ", "))
				}
				return nil
			},
		},
		{
			Name:         "Start",
			OnFailureTip: "govard doctor",
			Run: func() error {
				upArgs := []string{"up", "-d"}
				if context.RemoveOrphans {
					upArgs = append(upArgs, "--remove-orphans")
				}
				if context.ForceRecreate {
					upArgs = append(upArgs, "--force-recreate")
				}

				err := engine.RunCompose(cmd.Context(), engine.ComposeOptions{
					ProjectDir:  context.Cwd,
					ProjectName: context.Config.ProjectName,
					ComposeFile: context.Compose,
					Args:        upArgs,
					Stdout:      context.Out,
					Stderr:      context.Err,
				})
				if err != nil {
					if !context.FallbackLocal {
						return fmt.Errorf("docker compose up failed: %w", err)
					}

					pterm.Warning.Printf("docker compose up failed: %v\n", err)
					pterm.Info.Println("Attempting local Govard image build fallback...")

					built, fallbackErr := engine.FallbackBuildMissingGovardImagesFromCompose(context.Compose, context.Out, context.Err)
					if fallbackErr != nil {
						return fmt.Errorf("docker compose up failed: %w (local fallback failed: %v)", err, fallbackErr)
					}
					if len(built) == 0 {
						return fmt.Errorf("docker compose up failed: %w", err)
					}

					pterm.Success.Printf("Local fallback built %d image(s): %s\n", len(built), strings.Join(built, ", "))
					pterm.Info.Println("Retrying docker compose up after local fallback build...")

					retryErr := engine.RunCompose(cmd.Context(), engine.ComposeOptions{
						ProjectDir:  context.Cwd,
						ProjectName: context.Config.ProjectName,
						ComposeFile: context.Compose,
						Args:        upArgs,
						Stdout:      context.Out,
						Stderr:      context.Err,
					})
					if retryErr != nil {
						return fmt.Errorf("docker compose up failed after local fallback retry: %w", retryErr)
					}
				}
				return nil
			},
		},
		{
			Name:         "Ready",
			OnFailureTip: "govard env logs php -f",
			Run: func() error {
				return waitForUpRuntimeReadiness(context.Config, defaultUpReadinessTimeout)
			},
		},
		{
			Name:         "Verify",
			OnFailureTip: "govard doctor",
			Run: func() error {
				{
					if err := FixComposerCompatibility(context.Config); err != nil {
						pterm.Warning.Printf("Could not ensure Composer version: %v\n", err)
					}
				}

				{
					if context.Config.Framework == "wordpress" {
						if err := FixWordPressCompatibility(context.Config); err != nil {
							pterm.Warning.Printf("Could not ensure WordPress (WP-CLI) compatibility: %v\n", err)
						}
					}
				}

				target := ResolveUpProxyTarget(context.Config)
				allDomains := context.Config.AllDomains()

				var proxyErr error
				if len(allDomains) > 0 {
					proxyErr = proxy.RegisterDomains(allDomains, target)
				}

				if context.Config.Stack.Features.Search && len(allDomains) > 0 {
					searchTarget := ResolveUpSearchProxyTarget(context.Config)
					if searchErr := proxy.RegisterSearchDomains(allDomains, searchTarget); searchErr != nil {
						pterm.Warning.Printf("Could not register search proxy route: %v\n", searchErr)
					}
				}

				var wg sync.WaitGroup
				for _, domain := range allDomains {
					wg.Add(1)
					go func(d string) {
						defer wg.Done()
						if engine.IsDomainResolvableLocally(d) {
							pterm.Success.Printf("Domain %s already resolves locally\n", d)
						} else if err := engine.AddHostsEntry(d); err != nil {
							pterm.Warning.Printf("Could not update hosts file for %s: %v\n", d, err)
							pterm.Info.Printf("Please manually add '127.0.0.1 %s' to your hosts file.\n", d)
						} else {
							pterm.Success.Printf("Domain %s mapped to 127.0.0.1\n", d)
						}

						if proxyErr != nil {
							pterm.Warning.Printf("Could not register domain %s with Govard Proxy: %v\n", d, proxyErr)
						} else {
							pterm.Success.Printf("Domain %s registered with Govard Proxy -> %s\n", d, target)
						}
					}(domain)
				}
				wg.Wait()

				if err := engine.RunHooks(context.Config, engine.HookPostUp, context.Out, context.Err); err != nil {
					return fmt.Errorf("post-up hooks failed: %w", err)
				}

				if err := engine.RefreshPMAActiveProjects(); err != nil {
					pterm.Warning.Printf("Could not refresh PMA active projects: %v\n", err)
				}

				if engine.IsMagento2Family(context.Config.Framework) {
					frameworkName := engine.Magento2FamilyDisplayName(context.Config.Framework)
					if context.SkipTuning {
						pterm.Info.Println("Skipping framework auto-configuration (--no-tuning)")
					} else if context.ShiftInfo != nil && context.ShiftInfo.Shifted && stdinIsTerminal() {
						// Prompt user for Magento tuning when shift is detected
						pterm.Info.Printf("%s environment detected.\n", frameworkName)
						pterm.Info.Printf("Reason: %s\n", context.ShiftInfo.Reason)
						proceed, _ := pterm.DefaultInteractiveConfirm.
							WithDefaultValue(true).
							WithDefaultText("Y = tune now, N = skip tuning").
							Show(fmt.Sprintf("Run %s auto-configuration?", frameworkName))
						if proceed {
							if err := engine.ConfigureMagento(context.Config.ProjectName, context.Config, false, context.ShiftInfo); err != nil {
								pterm.Warning.Printf("%s auto-configuration failed: %v\n", frameworkName, err)
							}
						} else {
							pterm.Info.Printf("%s tuning skipped. Run 'govard config auto' to configure later.\n", frameworkName)
						}
					} else if context.ShiftInfo != nil && context.ShiftInfo.Shifted {
						// Non-interactive mode: run without prompt if shift detected
						if err := engine.ConfigureMagento(context.Config.ProjectName, context.Config, false, context.ShiftInfo); err != nil {
							pterm.Warning.Printf("%s auto-configuration failed: %v\n", frameworkName, err)
						}
					}
					// No shift detected: skip ConfigureMagento entirely
				}

				return nil
			},
		},
	}
}

func buildUpReadinessChecks(config engine.Config) []upReadinessCheck {
	if strings.TrimSpace(config.ProjectName) == "" {
		return nil
	}

	// Skip PHP readiness checks if PHP is not required (e.g., custom without PHP, node-based frameworks)
	if !engine.RequiresPHP(config) {
		return nil
	}

	phpFPMProbe := []string{
		"php",
		"-r",
		`$s=@fsockopen("127.0.0.1",9000,$errno,$errstr,1); if($s===false){fwrite(STDERR,$errno . ":" . $errstr); exit(1);} fclose($s);`,
	}

	checks := []upReadinessCheck{
		{
			Service:       "php",
			ContainerName: fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix),
			ProbeArgs:     phpFPMProbe,
		},
	}

	if config.Stack.Features.Xdebug {
		checks = append(checks, upReadinessCheck{
			Service:       "php-debug",
			ContainerName: fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPDebugSuffix),
			ProbeArgs:     phpFPMProbe,
		})
	}

	if config.Stack.Features.Varnish {
		checks = append(checks, upReadinessCheck{
			Service:       "varnish",
			ContainerName: fmt.Sprintf("%s%s", config.ProjectName, conventions.VarnishSuffix),
			ProbeArgs:     []string{"true"},
		})
	}

	if config.Stack.Features.Cache || config.Stack.Services.Cache != "none" {
		checks = append(checks, upReadinessCheck{
			Service:       "redis",
			ContainerName: fmt.Sprintf("%s%s", config.ProjectName, conventions.RedisSuffix),
			ProbeArgs:     []string{"redis-cli", "ping"},
		})
	}

	return checks
}

func readinessProbeAttempts(timeout time.Duration) int {
	if timeout <= 0 {
		return 1
	}
	if upReadinessProbeInterval <= 0 {
		return 1
	}

	attempts := int(timeout / upReadinessProbeInterval)
	if timeout%upReadinessProbeInterval != 0 {
		attempts++
	}
	if attempts < 1 {
		return 1
	}
	return attempts
}

func waitForUpRuntimeReadiness(config engine.Config, timeout time.Duration) error {
	checks := buildUpReadinessChecks(config)
	if len(checks) == 0 {
		return nil
	}

	maxAttempts := readinessProbeAttempts(timeout)
	for _, check := range checks {
		pterm.Info.Printf("Waiting for %s runtime readiness...\n", check.Service)

		var lastErr error
		abortConsecutiveCount := 0
		maxAbortTolerations := 3 // Tolerate transient exit states right after container creation/recreation

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			state, stateErr := inspectUpContainerState(check.ContainerName)
			if stateErr == nil && shouldAbortReadinessForContainerState(state) {
				abortConsecutiveCount++

				// If it crashed, try to kickstart it once (handles entrypoint identity change crashes)
				if abortConsecutiveCount == 1 {
					_ = exec.Command("docker", "start", check.ContainerName).Run()
				}

				if abortConsecutiveCount >= maxAbortTolerations {
					return fmt.Errorf("%s container %s is %s (exit code %s, oom=%s): %s", check.Service, check.ContainerName, state.Status, state.ExitCode, state.OOMKilled, engine.FirstNonEmpty(state.Error, "container stopped during startup"))
				}

				lastErr = fmt.Errorf("container is in state %s", state.Status)
			} else {
				abortConsecutiveCount = 0 // Reset if it's not in an abort state

				lastErr = upReadinessProbeRunner(check.ContainerName, check.ProbeArgs)
				if lastErr == nil {
					pterm.Success.Printf("%s runtime is ready\n", check.Service)
					break
				}
			}

			if attempt < maxAttempts {
				upReadinessSleep(upReadinessProbeInterval)
			}
		}

		if lastErr != nil {
			return fmt.Errorf("%s runtime did not become ready after %s: %w", check.Service, timeout, lastErr)
		}
	}

	return nil
}

func inspectUpContainerState(containerName string) (upContainerState, error) {
	raw, err := upContainerStateRunner(containerName)
	if err != nil {
		return upContainerState{}, err
	}

	parts := strings.SplitN(raw, "|", 4)
	for len(parts) < 4 {
		parts = append(parts, "")
	}

	return upContainerState{
		Status:    strings.TrimSpace(parts[0]),
		ExitCode:  strings.TrimSpace(parts[1]),
		OOMKilled: strings.TrimSpace(parts[2]),
		Error:     strings.TrimSpace(parts[3]),
	}, nil
}

func shouldAbortReadinessForContainerState(state upContainerState) bool {
	switch strings.ToLower(strings.TrimSpace(state.Status)) {
	case "exited", "dead", "removing":
		return true
	default:
		return false
	}
}

func runUpPipeline(stages []upPipelineStage) error {
	total := len(stages)
	for index, stage := range stages {
		step := fmt.Sprintf("[%d/%d] %s", index+1, total, stage.Name)
		pterm.Info.Printf("%s...\n", step)

		started := time.Now()
		if err := stage.Run(); err != nil {
			err = fmt.Errorf("%s failed (%s): %w", step, time.Since(started).Round(time.Millisecond), err)
			if stage.OnFailureTip != "" {
				pterm.Info.Printf("Suggested next command: %s\n", stage.OnFailureTip)
			}
			return err
		}
		pterm.Success.Printf("%s completed (%s)\n", step, time.Since(started).Round(time.Millisecond))
	}
	return nil
}

func refreshCrossProjectRuntimeHosts(ctx context.Context, currentProjectRoot string, currentConfig engine.Config, stdout, stderr io.Writer) error {
	currentProjectName := strings.TrimSpace(currentConfig.ProjectName)
	if currentProjectName == "" {
		return nil
	}

	runningProjects, err := upRefreshRunningProjectNames(ctx)
	if err != nil || len(runningProjects) == 0 {
		return err
	}

	entries, err := upRefreshReadProjectRegistryEntries()
	if err != nil {
		return err
	}

	entryByProjectName := make(map[string]engine.ProjectRegistryEntry, len(entries))
	for _, entry := range entries {
		projectName := strings.TrimSpace(entry.ProjectName)
		if projectName == "" {
			continue
		}
		entryByProjectName[projectName] = entry
	}

	cleanCurrentRoot := filepath.Clean(strings.TrimSpace(currentProjectRoot))
	var refreshErrors []string

	for _, runningProjectName := range runningProjects {
		projectName := strings.TrimSpace(runningProjectName)
		if projectName == "" || projectName == currentProjectName {
			continue
		}

		entry, ok := entryByProjectName[projectName]
		if !ok {
			continue
		}

		projectRoot := filepath.Clean(strings.TrimSpace(entry.Path))
		if projectRoot == "" || projectRoot == cleanCurrentRoot {
			continue
		}

		projectConfig, _, loadErr := upRefreshLoadConfigFromDir(projectRoot, false)
		if loadErr != nil {
			refreshErrors = append(refreshErrors, fmt.Sprintf("%s load config: %v", projectName, loadErr))
			continue
		}

		// Targeted refresh: Only restart if the running project has explicitly
		// declared that it needs to see the current project.
		if !projectDependsOn(projectConfig, currentProjectName) {
			continue
		}

		frameworkConfig, ok := engine.GetFrameworkConfig(projectConfig.Framework)
		if !ok || frameworkConfig.Runtime != "php" {
			continue
		}

		if renderErr := upRefreshRenderBlueprint(projectRoot, projectConfig); renderErr != nil {
			refreshErrors = append(refreshErrors, fmt.Sprintf("%s render blueprint: %v", projectName, renderErr))
			continue
		}

		pterm.Info.Printf("Refreshing cross-project dependencies for %s...\n", projectName)

		args := []string{"up", "-d", "--no-deps", "php"}
		if projectConfig.Stack.Features.Xdebug {
			args = append(args, "php-debug")
		}
		composePath := engine.ComposeFilePathWithProfile(projectRoot, projectConfig.ProjectName, projectConfig.Profile)
		if composeErr := upRefreshRunCompose(ctx, engine.ComposeOptions{
			ProjectDir:  projectRoot,
			ProjectName: projectConfig.ProjectName,
			ComposeFile: composePath,
			Args:        args,
			Stdout:      stdout,
			Stderr:      stderr,
		}); composeErr != nil {
			refreshErrors = append(refreshErrors, fmt.Sprintf("%s refresh runtime hosts: %v", projectName, composeErr))
		}
	}

	if len(refreshErrors) > 0 {
		return fmt.Errorf("failed updates: %s", strings.Join(refreshErrors, "; "))
	}

	return nil
}

func projectDependsOn(config engine.Config, projectName string) bool {
	for _, host := range config.LinkedProjects {
		if strings.TrimSpace(host) == projectName {
			return true
		}
	}
	return false
}

// ResolveUpProxyTarget resolves the upstream container for proxy registration.
func ResolveUpProxyTarget(config engine.Config) string {
	target := config.ProjectName + "-web" + conventions.ReplicaSuffix
	if config.Stack.Features.Varnish {
		target = config.ProjectName + conventions.VarnishSuffix
	}
	return target
}

// ResolveUpSearchProxyTarget resolves the upstream container for the search-engine proxy route.
func ResolveUpSearchProxyTarget(config engine.Config) string {
	return config.ProjectName + conventions.ElasticsearchSuffix
}

// ApplyQuickstartProfile trims optional runtime services for faster first startup.
func ApplyQuickstartProfile(config *engine.Config) {
	if config == nil {
		return
	}

	config.Stack.Features.Xdebug = false
	config.Stack.Features.Varnish = false

	config.Stack.Services.Search = "none"
	config.Stack.SearchVersion = ""
	config.Stack.Features.Search = false

	config.Stack.Services.Cache = "none"
	config.Stack.CacheVersion = ""
	config.Stack.Features.Cache = false

	config.Stack.Services.Queue = "none"
	config.Stack.QueueVersion = ""
}

// CheckMagentoRuntimeSync checks the detected Magento framework against configured
// runtime and returns warnings if there's a mismatch (without auto-tuning).
func CheckMagentoRuntimeSync(config engine.Config, metadata engine.ProjectMetadata) []string {
	if config.Framework != "magento2" {
		return nil
	}

	cwd, _ := os.Getwd()
	rawConfig, err := engine.LoadRawConfigFromDir(cwd, false)
	if err != nil {
		return nil
	}

	warnings := engine.CollectProfileSyncWarnings(rawConfig, metadata)
	if len(warnings) > 0 {
		version := strings.TrimSpace(metadata.Version)
		if version == "" {
			version = strings.TrimSpace(config.FrameworkVersion)
		}
		return []string{fmt.Sprintf(
			"Magento %s expects different services: %s. Run 'govard doctor --fix' to align.",
			version, strings.Join(warnings, ", "),
		)}
	}

	return nil
}

func compareNumericDotVersions(left, right string) (int, bool) {
	return engine.CompareNumericDotVersions(left, right)
}

func runUpCommand(cmd *cobra.Command, args []string) (err error) {
	updater.CheckForUpdates(Version)
	startedAt := time.Now()

	quickstart, _ := cmd.Flags().GetBool("quickstart")
	explicitProfile, _ := cmd.Flags().GetString("profile")
	pull, _ := cmd.Flags().GetBool("pull")
	fallbackLocalBuild := boolFlagOrDefault(cmd, "fallback-local-build", true)
	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	forceRecreate, _ := cmd.Flags().GetBool("force-recreate")
	updateLock, _ := cmd.Flags().GetBool("update-lock")
	skipTuning, _ := cmd.Flags().GetBool("no-tuning")
	cwd, _ := os.Getwd()
	// Resolve profile: explicit flag > project registry (last-used) > empty (default)
	profile := engine.ResolveEffectiveProfile(cwd, explicitProfile)
	context := upRuntimeContext{
		Cwd:           cwd,
		Profile:       profile,
		Quickstart:    quickstart,
		Pull:          pull,
		FallbackLocal: fallbackLocalBuild,
		RemoveOrphans: removeOrphans,
		ForceRecreate: forceRecreate,
		UpdateLock:    updateLock,
		SkipTuning:    skipTuning,
		Out:           cmd.OutOrStdout(),
		Err:           cmd.ErrOrStderr(),
	}
	defer func() {
		status := engine.OperationStatusSuccess
		message := "up completed"
		if err != nil {
			status = engine.OperationStatusFailure
			message = err.Error()
		}
		writeOperationEventBestEffort(
			"up.run",
			status,
			context.Config,
			"",
			"",
			message,
			"",
			time.Since(startedAt),
		)
	}()
	stages := buildUpPipelineStages(cmd, &context)
	if err = runUpPipeline(stages); err != nil {
		return err
	}
	trackProjectRegistryBestEffort(context.Config, cwd, "up")
	if refreshErr := refreshCrossProjectRuntimeHosts(cmd.Context(), cwd, context.Config, context.Out, context.Err); refreshErr != nil {
		pterm.Warning.Printf("Could not refresh cross-project runtime hosts: %v\n", refreshErr)
	}
	pterm.Success.Printf("Environment is up and running at https://%s\n", context.Config.Domain)
	return nil
}

func ensureUpProfileResourceSync(context *upRuntimeContext) error {
	if context.Profile == "" {
		return nil
	}

	if context.Config.Stack.Services.DB == "" || context.Config.Stack.Services.DB == "none" {
		return nil
	}

	targetVolume := fmt.Sprintf("%s_db-data-%s", context.Config.ProjectName, context.Profile)
	isEmpty, err := engine.IsVolumeEmpty(targetVolume)
	if err != nil {
		return fmt.Errorf("check target volume: %w", err)
	}

	if !isEmpty {
		return nil
	}

	// Target is empty, look for a source volume from default profile
	sourceVolume := fmt.Sprintf("%s_db-data", context.Config.ProjectName)
	sourceEmpty, err := engine.IsVolumeEmpty(sourceVolume)
	if err != nil || sourceEmpty {
		return nil // No source data found, nothing to clone
	}

	pterm.Info.Printf("Detected empty database volume for profile %q, but found data in default profile.\n", context.Profile)
	proceed, err := pterm.DefaultInteractiveConfirm.
		WithDefaultValue(true).
		Show(fmt.Sprintf("Clone data from '%s' to '%s'?", sourceVolume, targetVolume))

	if err != nil || !proceed {
		return nil
	}

	pterm.Info.Printf("Cloning database data...\n")

	// Ensure target volume exists with correct labels
	checkTarget := exec.Command("docker", "volume", "inspect", targetVolume)
	if err := checkTarget.Run(); err != nil {
		composeVolumeName := fmt.Sprintf("db-data-%s", context.Profile)
		createVol := exec.Command("docker", "volume", "create",
			"--name", targetVolume,
			"--label", fmt.Sprintf("com.docker.compose.project=%s", context.Config.ProjectName),
			"--label", fmt.Sprintf("com.docker.compose.volume=%s", composeVolumeName),
		)
		if err := createVol.Run(); err != nil {
			return fmt.Errorf("failed to create target volume: %w", err)
		}
	}

	if err := engine.CloneVolume(sourceVolume, targetVolume); err != nil {
		return fmt.Errorf("volume clone failed: %w", err)
	}

	pterm.Success.Println("Database volume successfully cloned!")
	return nil
}

func addUpFlags(command *cobra.Command) {
	command.Flags().Bool("quickstart", false, "Use a minimal runtime profile for faster first run")
	command.Flags().String("profile", "", "Environment scope (profile) to use")
	command.Flags().Bool("pull", false, "Pull latest images before starting")
	command.Flags().Bool("fallback-local-build", true, "When pull/start fails due missing Govard images, build missing Govard-managed images locally and retry")
	command.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the compose file")
	command.Flags().Bool("force-recreate", false, "Recreate containers even if their configuration and image haven't changed")
	command.Flags().Bool("update-lock", false, "Automatically update govard.lock if mismatches are found")
	command.Flags().Bool("no-tuning", false, "Skip framework auto-configuration after environment starts")
}

func init() {
	addUpFlags(upCmd)
}
