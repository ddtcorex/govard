package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
	"govard/internal/proxy"

	"github.com/spf13/cobra"
)

const frontendReadinessTimeout = 90 * time.Second

var frontendContainerStateRunner = inspectFrontendContainerState

// FrontendDependenciesForTest isolates the generic command orchestration from
// framework implementations and Docker for command tests.
type FrontendDependenciesForTest struct {
	DiscoverFrontendSyncRuntimeForFramework func(string) (types.FrontendSyncDiscoverer, bool)
	RenderFrontendBlueprintForFramework     func(string) (types.FrontendSyncRenderer, bool)
	IsContainerRunning                      func(context.Context, string) bool
	RunCompose                              func(context.Context, engine.ComposeOptions) error
	WaitForRuntimeReadiness                 func(context.Context, engine.Config, types.FrontendSyncRuntime) error
	RegisterFrontendProxy                   func(engine.Config, types.FrontendSyncRuntime) error
	UnregisterFrontendProxy                 func(string) error
}

var frontendDeps = FrontendDependenciesForTest{
	DiscoverFrontendSyncRuntimeForFramework: frameworks.FrontendSyncDiscoverer,
	RenderFrontendBlueprintForFramework:     frameworks.FrontendSyncRenderer,
	IsContainerRunning:                      engine.IsContainerRunning,
	RunCompose:                              engine.RunCompose,
	WaitForRuntimeReadiness:                 waitForFrontendRuntimeReadiness,
	RegisterFrontendProxy:                   registerFrontendProxy,
	UnregisterFrontendProxy:                 proxy.UnregisterFrontend,
}

var frontendCmd = &cobra.Command{
	Use:   "frontend",
	Short: "Control the project-owned frontend development runtime",
}

var frontendStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the frontend development runtime",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get project directory: %w", err)
		}
		return runFrontendStart(cmd.Context(), root, config, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

var frontendStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Remove only frontend development services",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get project directory: %w", err)
		}
		return runFrontendStop(cmd.Context(), root, config, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

var frontendLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Stream logs from a frontend development service",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get project directory: %w", err)
		}
		service := ""
		if len(args) == 1 {
			service = args[0]
		}
		follow, _ := cmd.Flags().GetBool("follow")
		return runFrontendLogs(cmd.Context(), root, config, service, follow, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

func runFrontendStart(ctx context.Context, root string, config engine.Config, stdout, stderr io.Writer) error {
	_, render, err := frontendProvider(config)
	if err != nil {
		return err
	}
	if err := requireFrontendBackend(ctx, config, "start"); err != nil {
		return err
	}
	runtime, composePath, err := render(root, config)
	if err != nil {
		return fmt.Errorf("render frontend runtime: %w", err)
	}
	if err := validateFrontendRuntime(runtime); err != nil {
		return err
	}
	if err := frontendDeps.RunCompose(ctx, frontendComposeOptions(root, config, composePath, []string{"up", "-d"}, stdout, stderr)); err != nil {
		return fmt.Errorf("start frontend services: %w", err)
	}
	if err := frontendDeps.WaitForRuntimeReadiness(ctx, config, runtime); err != nil {
		return fmt.Errorf("frontend %s runtime did not become ready: %w", runtime.Mode, err)
	}
	if err := frontendDeps.RegisterFrontendProxy(config, runtime); err != nil {
		cleanupErr := frontendDeps.RunCompose(ctx, frontendComposeOptions(root, config, composePath, []string{"down"}, stdout, stderr))
		if cleanupErr != nil {
			return fmt.Errorf("register frontend proxy: %w (also failed to stop frontend services: %v)", err, cleanupErr)
		}
		return fmt.Errorf("register frontend proxy: %w", err)
	}
	return nil
}

func runFrontendStop(ctx context.Context, root string, config engine.Config, stdout, stderr io.Writer) error {
	if _, _, err := frontendProvider(config); err != nil {
		return err
	}
	if err := requireFrontendBackend(ctx, config, "stop"); err != nil {
		return err
	}
	if err := frontendDeps.UnregisterFrontendProxy(config.ProjectName); err != nil {
		return fmt.Errorf("remove frontend proxy registration: %w", err)
	}
	composePath := engine.FrontendComposeFilePath(root, config.ProjectName, config.Profile)
	if _, err := os.Stat(composePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect frontend compose file: %w", err)
	}
	// Deliberately omit -v: a frontend restart must retain its lock-keyed
	// dependency volumes and must not touch application services or volumes.
	if err := frontendDeps.RunCompose(ctx, frontendComposeOptions(root, config, composePath, []string{"down"}, stdout, stderr)); err != nil {
		return fmt.Errorf("stop frontend services: %w", err)
	}
	return nil
}

func runFrontendLogs(ctx context.Context, root string, config engine.Config, requestedService string, follow bool, stdout, stderr io.Writer) error {
	discover, _, err := frontendProvider(config)
	if err != nil {
		return err
	}
	if err := requireFrontendBackend(ctx, config, "logs"); err != nil {
		return err
	}
	runtime, err := discover(root)
	if err != nil {
		return fmt.Errorf("discover frontend runtime: %w", err)
	}
	if err := validateFrontendRuntime(runtime); err != nil {
		return err
	}
	service := strings.TrimSpace(requestedService)
	if service == "" {
		service = "sync"
		if _, err := fmt.Fprintf(stdout, "Available frontend services: %s\nShowing logs for primary service %s\n", strings.Join(runtime.Services, ", "), service); err != nil {
			return fmt.Errorf("write frontend log selection: %w", err)
		}
	}
	if !frontendServiceExists(runtime.Services, service) {
		return fmt.Errorf("frontend service %q is not available; choose one of: %s", service, strings.Join(runtime.Services, ", "))
	}
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, service)
	composePath := engine.FrontendComposeFilePath(root, config.ProjectName, config.Profile)
	if err := frontendDeps.RunCompose(ctx, frontendComposeOptions(root, config, composePath, args, stdout, stderr)); err != nil {
		return fmt.Errorf("stream frontend logs: %w", err)
	}
	return nil
}

func requireFrontendBackend(ctx context.Context, config engine.Config, operation string) error {
	webContainer := fmt.Sprintf("%s%s", config.ProjectName, conventions.WebSuffix)
	if !frontendDeps.IsContainerRunning(ctx, webContainer) {
		return fmt.Errorf("frontend %s requires the backend web container %q to be running; run `govard env up` first", operation, webContainer)
	}
	return nil
}

func frontendProvider(config engine.Config) (types.FrontendSyncDiscoverer, types.FrontendSyncRenderer, error) {
	if !config.Stack.Features.FrontendSync {
		return nil, nil, fmt.Errorf("frontend sync is disabled; enable stack.features.frontend_sync before using `govard frontend`")
	}
	discover, ok := frontendDeps.DiscoverFrontendSyncRuntimeForFramework(config.Framework)
	if !ok {
		return nil, nil, fmt.Errorf("frontend sync is not supported for framework %q", config.Framework)
	}
	render, ok := frontendDeps.RenderFrontendBlueprintForFramework(config.Framework)
	if !ok {
		return nil, nil, fmt.Errorf("frontend sync is not supported for framework %q", config.Framework)
	}
	return discover, render, nil
}

func frontendComposeOptions(root string, config engine.Config, composePath string, args []string, stdout, stderr io.Writer) engine.ComposeOptions {
	return engine.ComposeOptions{
		ProjectDir:  root,
		ProjectName: frontendComposeProjectName(config.ProjectName),
		ComposeFile: composePath,
		Args:        args,
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       os.Stdin,
	}
}

func registerFrontendProxy(config engine.Config, runtime types.FrontendSyncRuntime) error {
	return proxy.RegisterFrontend(frontendProxyRegistration(config, runtime))
}

func frontendProxyRegistration(config engine.Config, runtime types.FrontendSyncRuntime) proxy.FrontendRegistration {
	endpoint := runtime.PublicEndpoint
	target := frontendRuntimeTarget(config.ProjectName, endpoint.Service, endpoint.Port)
	registration := proxy.FrontendRegistration{
		ProjectName: config.ProjectName,
		Domains:     config.AllDomains(),
		Endpoint: proxy.FrontendEndpoint{
			Path:        endpoint.Path,
			StripPrefix: endpoint.StripPrefix,
			Target:      target,
		},
	}
	if runtime.HTMLInjection != nil {
		registration.HTMLInjectionTarget = frontendRuntimeTarget(
			config.ProjectName,
			runtime.HTMLInjection.Service,
			runtime.HTMLInjection.Port,
		)
	}
	return registration
}

func frontendRuntimeTarget(projectName, service string, port int) string {
	return fmt.Sprintf("%s:%d", frontendRuntimeContainerName(projectName, service), port)
}

func frontendComposeProjectName(projectName string) string {
	return strings.TrimSpace(projectName) + "-frontend"
}

func frontendRuntimeContainerName(projectName, service string) string {
	return fmt.Sprintf("%s-%s-1", frontendComposeProjectName(projectName), service)
}

func validateFrontendRuntime(runtime types.FrontendSyncRuntime) error {
	if strings.TrimSpace(runtime.Mode) == "" {
		return fmt.Errorf("frontend provider returned a runtime without a mode")
	}
	if !frontendServiceExists(runtime.Services, "sync") {
		return fmt.Errorf("frontend provider runtime %q does not declare its sync service", runtime.Mode)
	}
	return nil
}

func frontendServiceExists(services []string, wanted string) bool {
	for _, service := range services {
		if service == wanted {
			return true
		}
	}
	return false
}

func waitForFrontendRuntimeReadiness(ctx context.Context, config engine.Config, runtime types.FrontendSyncRuntime) error {
	for _, service := range runtime.Services {
		container := frontendRuntimeContainerName(config.ProjectName, service)
		deadline := time.Now().Add(frontendReadinessTimeout)
		var lastState string
		for time.Now().Before(deadline) {
			state, err := frontendContainerStateRunner(ctx, container)
			if err == nil {
				lastState = state
				if state == "healthy" {
					break
				}
				if state == "exited" || state == "dead" {
					return fmt.Errorf("container %s is %s; run `govard frontend logs %s -f`", container, state, service)
				}
			}
			time.Sleep(1500 * time.Millisecond)
		}
		if lastState != "healthy" {
			return fmt.Errorf("container %s health is %s", container, engine.FirstNonEmpty(lastState, "unknown"))
		}
	}
	return nil
}

func inspectFrontendContainerState(ctx context.Context, container string) (string, error) {
	command := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", container)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(output))), nil
}

// SetFrontendDependenciesForTest swaps generic frontend lifecycle seams.
func SetFrontendDependenciesForTest(dependencies FrontendDependenciesForTest) func() {
	previous := frontendDeps
	if dependencies.DiscoverFrontendSyncRuntimeForFramework != nil {
		frontendDeps.DiscoverFrontendSyncRuntimeForFramework = dependencies.DiscoverFrontendSyncRuntimeForFramework
	}
	if dependencies.RenderFrontendBlueprintForFramework != nil {
		frontendDeps.RenderFrontendBlueprintForFramework = dependencies.RenderFrontendBlueprintForFramework
	}
	if dependencies.IsContainerRunning != nil {
		frontendDeps.IsContainerRunning = dependencies.IsContainerRunning
	}
	if dependencies.RunCompose != nil {
		frontendDeps.RunCompose = dependencies.RunCompose
	}
	if dependencies.WaitForRuntimeReadiness != nil {
		frontendDeps.WaitForRuntimeReadiness = dependencies.WaitForRuntimeReadiness
	}
	if dependencies.RegisterFrontendProxy != nil {
		frontendDeps.RegisterFrontendProxy = dependencies.RegisterFrontendProxy
	}
	if dependencies.UnregisterFrontendProxy != nil {
		frontendDeps.UnregisterFrontendProxy = dependencies.UnregisterFrontendProxy
	}
	return func() { frontendDeps = previous }
}

// RunFrontendStartForTest invokes the start lifecycle without configuration I/O.
func RunFrontendStartForTest(ctx context.Context, root string, config engine.Config) error {
	return runFrontendStart(ctx, root, config, io.Discard, io.Discard)
}

// RunFrontendStopForTest invokes the stop lifecycle without configuration I/O.
func RunFrontendStopForTest(ctx context.Context, root string, config engine.Config) error {
	return runFrontendStop(ctx, root, config, io.Discard, io.Discard)
}

// RunFrontendLogsForTest invokes the logs lifecycle without configuration I/O.
func RunFrontendLogsForTest(ctx context.Context, root string, config engine.Config, service string, follow bool) error {
	return runFrontendLogs(ctx, root, config, service, follow, io.Discard, io.Discard)
}

// RunFrontendLogsWithWritersForTest invokes log selection with observable output.
func RunFrontendLogsWithWritersForTest(ctx context.Context, root string, config engine.Config, service string, follow bool, stdout, stderr io.Writer) error {
	return runFrontendLogs(ctx, root, config, service, follow, stdout, stderr)
}

// SetFrontendContainerStateRunnerForTest replaces frontend health inspection.
func SetFrontendContainerStateRunnerForTest(fn func(context.Context, string) (string, error)) func() {
	previous := frontendContainerStateRunner
	frontendContainerStateRunner = fn
	return func() { frontendContainerStateRunner = previous }
}

// WaitForFrontendRuntimeReadinessForTest exposes readiness for every provider service.
func WaitForFrontendRuntimeReadinessForTest(ctx context.Context, config engine.Config, runtime types.FrontendSyncRuntime) error {
	return waitForFrontendRuntimeReadiness(ctx, config, runtime)
}

// FrontendProxyRegistrationForTest exposes framework-neutral target naming.
func FrontendProxyRegistrationForTest(config engine.Config, runtime types.FrontendSyncRuntime) proxy.FrontendRegistration {
	return frontendProxyRegistration(config, runtime)
}

func init() {
	frontendLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	frontendCmd.AddCommand(frontendStartCmd, frontendStopCmd, frontendLogsCmd)
}
