package tests

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

type fakeFrontendSyncProvider struct {
	runtime     types.FrontendSyncRuntime
	composePath string
	rendered    bool
	discovered  bool
}

func (provider *fakeFrontendSyncProvider) DiscoverFrontendSyncRuntime(string) (types.FrontendSyncRuntime, error) {
	provider.discovered = true
	return provider.runtime, nil
}

func (provider *fakeFrontendSyncProvider) RenderFrontendBlueprint(string, engine.Config) (types.FrontendSyncRuntime, string, error) {
	provider.rendered = true
	return provider.runtime, provider.composePath, nil
}

func TestFrontendStartUsesRegisteredProviderAndDedicatedCompose(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{
		runtime: types.FrontendSyncRuntime{
			Mode:     "fake",
			Services: []string{"sync", "watch-theme"},
			PublicEndpoint: types.FrontendSyncPublicEndpoint{
				Path:    "/frontend-client/*",
				Service: "sync",
				Port:    3000,
			},
		},
		composePath: "/generated/frontend.yml",
	}

	var composeArgs []string
	var readyRuntime types.FrontendSyncRuntime
	var registeredRuntime types.FrontendSyncRuntime
	events := []string{}
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(name string) (types.FrontendSyncDiscoverer, bool) {
			if name != "fake-framework" {
				t.Fatalf("discoverer lookup framework = %q", name)
			}
			return provider.DiscoverFrontendSyncRuntime, true
		},
		RenderFrontendBlueprintForFramework: func(name string) (types.FrontendSyncRenderer, bool) {
			if name != "fake-framework" {
				t.Fatalf("renderer lookup framework = %q", name)
			}
			return provider.RenderFrontendBlueprint, true
		},
		IsContainerRunning: func(_ context.Context, name string) bool {
			return name == "sample-web-1"
		},
		RunCompose: func(_ context.Context, options engine.ComposeOptions) error {
			if options.ProjectDir != root || options.ProjectName != "sample-frontend" || options.ProjectName == config.ProjectName || options.ComposeFile != provider.composePath {
				t.Fatalf("compose options = %#v", options)
			}
			composeArgs = append([]string(nil), options.Args...)
			events = append(events, "compose-up")
			return nil
		},
		WaitForRuntimeReadiness: func(_ context.Context, _ engine.Config, runtime types.FrontendSyncRuntime) error {
			readyRuntime = runtime
			events = append(events, "healthy")
			return nil
		},
		RegisterFrontendProxy: func(gotConfig engine.Config, runtime types.FrontendSyncRuntime) error {
			if gotConfig.ProjectName != config.ProjectName {
				t.Fatalf("proxy config = %#v", gotConfig)
			}
			registeredRuntime = runtime
			events = append(events, "proxy-register")
			return nil
		},
	})
	defer restore()

	if err := cmd.RunFrontendStartForTest(context.Background(), root, config); err != nil {
		t.Fatalf("frontend start: %v", err)
	}
	if !provider.rendered {
		t.Fatal("frontend start did not render through the registered provider")
	}
	if got, want := composeArgs, []string{"up", "-d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compose args = %#v, want %#v", got, want)
	}
	if readyRuntime.Mode != "fake" {
		t.Fatalf("readiness runtime = %#v", readyRuntime)
	}
	if registeredRuntime.PublicEndpoint.Path != "/frontend-client/*" {
		t.Fatalf("registered runtime = %#v", registeredRuntime)
	}
	if got, want := events, []string{"compose-up", "healthy", "proxy-register"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frontend start events = %#v, want %#v", got, want)
	}
}

func TestFrontendStartStopsComposeWhenProxyRegistrationFails(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{
		runtime: types.FrontendSyncRuntime{
			Mode:     "fake",
			Services: []string{"sync"},
			PublicEndpoint: types.FrontendSyncPublicEndpoint{
				Path:    "/frontend-client/*",
				Service: "sync",
				Port:    3000,
			},
		},
		composePath: "/generated/frontend.yml",
	}
	var composeCalls [][]string
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return true },
		RunCompose: func(_ context.Context, options engine.ComposeOptions) error {
			if options.ProjectName != "sample-frontend" || options.ProjectName == config.ProjectName {
				t.Fatalf("cleanup Compose project = %q, want isolated frontend project", options.ProjectName)
			}
			composeCalls = append(composeCalls, append([]string(nil), options.Args...))
			return nil
		},
		WaitForRuntimeReadiness: func(context.Context, engine.Config, types.FrontendSyncRuntime) error { return nil },
		RegisterFrontendProxy: func(engine.Config, types.FrontendSyncRuntime) error {
			return errors.New("synthetic Caddy failure")
		},
	})
	defer restore()

	err := cmd.RunFrontendStartForTest(context.Background(), root, config)
	if err == nil || !strings.Contains(err.Error(), "synthetic Caddy failure") {
		t.Fatalf("frontend start error = %v", err)
	}
	if got, want := composeCalls, [][]string{{"up", "-d"}, {"down"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compose calls = %#v, want cleanup %#v", got, want)
	}
}

func TestFrontendStartRequiresRunningBackendWebContainer(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{
		runtime:     types.FrontendSyncRuntime{Mode: "fake", Services: []string{"sync"}},
		composePath: "/generated/frontend.yml",
	}

	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return false },
		RunCompose: func(context.Context, engine.ComposeOptions) error {
			t.Fatal("frontend start must not invoke Compose without a backend web container")
			return nil
		},
	})
	defer restore()

	err := cmd.RunFrontendStartForTest(context.Background(), root, config)
	if err == nil || !strings.Contains(err.Error(), "govard env up") {
		t.Fatalf("frontend start error = %v, want backend startup guidance", err)
	}
	if provider.rendered {
		t.Fatal("frontend start rendered before confirming backend web availability")
	}
}

func TestFrontendStopRemovesOnlyDedicatedFrontendServicesWithoutVolumes(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{}
	composePath := engine.FrontendComposeFilePath(root, config.ProjectName, config.Profile)
	if err := os.MkdirAll(filepath.Dir(composePath), 0o755); err != nil {
		t.Fatalf("create frontend compose directory: %v", err)
	}
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write frontend compose file: %v", err)
	}

	var composeArgs []string
	events := []string{}
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return true },
		UnregisterFrontendProxy: func(projectName string) error {
			if projectName != config.ProjectName {
				t.Fatalf("unregister project = %q", projectName)
			}
			events = append(events, "proxy-remove")
			return nil
		},
		RunCompose: func(_ context.Context, options engine.ComposeOptions) error {
			if options.ComposeFile != composePath {
				t.Fatalf("stop compose file = %q", options.ComposeFile)
			}
			if options.ProjectName != "sample-frontend" || options.ProjectName == config.ProjectName {
				t.Fatalf("stop Compose project = %q, want isolated frontend project", options.ProjectName)
			}
			composeArgs = append([]string(nil), options.Args...)
			events = append(events, "compose-down")
			return nil
		},
	})
	defer restore()

	if err := cmd.RunFrontendStopForTest(context.Background(), root, config); err != nil {
		t.Fatalf("frontend stop: %v", err)
	}
	if got, want := composeArgs, []string{"down"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compose args = %#v, want %#v", got, want)
	}
	if got, want := events, []string{"proxy-remove", "compose-down"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frontend stop events = %#v, want %#v", got, want)
	}
}

func TestFrontendStopIsNoOpWhenDedicatedComposeFileIsAbsent(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{}

	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return true },
		UnregisterFrontendProxy: func(projectName string) error {
			if projectName != config.ProjectName {
				t.Fatalf("unregister project = %q", projectName)
			}
			return nil
		},
		RunCompose: func(context.Context, engine.ComposeOptions) error {
			t.Fatal("frontend stop must not invoke Compose when its dedicated file is absent")
			return nil
		},
	})
	defer restore()

	if err := cmd.RunFrontendStopForTest(context.Background(), root, config); err != nil {
		t.Fatalf("frontend stop: %v", err)
	}
}

func TestFrontendStopAndLogsRequireRunningBackendWebContainer(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{runtime: types.FrontendSyncRuntime{
		Mode:     "fake",
		Services: []string{"sync"},
	}}

	var unregistered bool
	var composed bool
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return false },
		UnregisterFrontendProxy: func(string) error {
			unregistered = true
			return nil
		},
		RunCompose: func(context.Context, engine.ComposeOptions) error {
			composed = true
			return nil
		},
	})
	defer restore()

	for _, run := range []struct {
		name string
		fn   func() error
	}{
		{name: "stop", fn: func() error { return cmd.RunFrontendStopForTest(context.Background(), root, config) }},
		{name: "logs", fn: func() error { return cmd.RunFrontendLogsForTest(context.Background(), root, config, "", false) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			err := run.fn()
			if err == nil || !strings.Contains(err.Error(), "govard env up") || !strings.Contains(err.Error(), "sample-web-1") {
				t.Fatalf("frontend %s error = %v, want stopped-backend remediation", run.name, err)
			}
		})
	}
	if provider.discovered || unregistered || composed {
		t.Fatalf("stopped backend caused frontend side effects: discovered=%t unregistered=%t composed=%t", provider.discovered, unregistered, composed)
	}
}

func TestFrontendReadinessFailsWhenWatcherExits(t *testing.T) {
	config := frontendCommandConfig()
	runtime := types.FrontendSyncRuntime{
		Mode:     "fake",
		Services: []string{"sync", "watch-theme"},
	}

	restore := cmd.SetFrontendContainerStateRunnerForTest(func(_ context.Context, container string) (string, error) {
		switch container {
		case "sample-frontend-sync-1":
			return "healthy", nil
		case "sample-frontend-watch-theme-1":
			return "exited", nil
		default:
			t.Fatalf("unexpected container inspection %q", container)
			return "", nil
		}
	})
	defer restore()

	err := cmd.WaitForFrontendRuntimeReadinessForTest(context.Background(), config, runtime)
	if err == nil || !strings.Contains(err.Error(), "sample-frontend-watch-theme-1") {
		t.Fatalf("watcher readiness error = %v", err)
	}
}

func TestFrontendLogsRejectsApplicationServicesAndTargetsDiscoveredFrontendService(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{runtime: types.FrontendSyncRuntime{
		Mode:     "fake",
		Services: []string{"sync", "watch-theme"},
	}}

	var composeArgs []string
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return true },
		RunCompose: func(_ context.Context, options engine.ComposeOptions) error {
			if options.ProjectName != "sample-frontend" || options.ProjectName == config.ProjectName {
				t.Fatalf("logs Compose project = %q, want isolated frontend project", options.ProjectName)
			}
			composeArgs = append([]string(nil), options.Args...)
			return nil
		},
	})
	defer restore()

	if err := cmd.RunFrontendLogsForTest(context.Background(), root, config, "watch-theme", true); err != nil {
		t.Fatalf("frontend logs: %v", err)
	}
	if !provider.discovered {
		t.Fatal("frontend logs did not discover services through the registered provider")
	}
	if got, want := composeArgs, []string{"logs", "-f", "watch-theme"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compose args = %#v, want %#v", got, want)
	}

	if err := cmd.RunFrontendLogsForTest(context.Background(), root, config, "web", false); err == nil {
		t.Fatal("frontend logs accepted application service web")
	}
}

func TestFrontendProxyTargetsUseDistinctFrontendComposeProjectContainers(t *testing.T) {
	config := frontendCommandConfig()
	runtime := types.FrontendSyncRuntime{
		Mode:     "fake",
		Services: []string{"sync", "inject"},
		PublicEndpoint: types.FrontendSyncPublicEndpoint{
			Path:    "/frontend/*",
			Service: "sync",
			Port:    3000,
		},
		HTMLInjection: &types.FrontendSyncHTMLInjection{Service: "inject", Port: 3000},
	}

	registration := cmd.FrontendProxyRegistrationForTest(config, runtime)
	if registration.ProjectName != config.ProjectName {
		t.Fatalf("proxy route owner = %q, want application project %q", registration.ProjectName, config.ProjectName)
	}
	if got, want := registration.Endpoint.Target, "sample-frontend-sync-1:3000"; got != want {
		t.Fatalf("frontend endpoint target = %q, want %q", got, want)
	}
	if got, want := registration.HTMLInjectionTarget, "sample-frontend-inject-1:3000"; got != want {
		t.Fatalf("frontend injection target = %q, want %q", got, want)
	}
}

func TestFrontendLogsWithoutServiceListsAvailableServicesThenTargetsPrimary(t *testing.T) {
	root := t.TempDir()
	config := frontendCommandConfig()
	provider := &fakeFrontendSyncProvider{runtime: types.FrontendSyncRuntime{
		Mode:     "fake",
		Services: []string{"sync", "watch-theme", "inject"},
	}}

	var stdout bytes.Buffer
	var composeArgs []string
	restore := cmd.SetFrontendDependenciesForTest(cmd.FrontendDependenciesForTest{
		DiscoverFrontendSyncRuntimeForFramework: func(string) (types.FrontendSyncDiscoverer, bool) { return provider.DiscoverFrontendSyncRuntime, true },
		RenderFrontendBlueprintForFramework:     func(string) (types.FrontendSyncRenderer, bool) { return provider.RenderFrontendBlueprint, true },
		IsContainerRunning:                      func(context.Context, string) bool { return true },
		RunCompose: func(_ context.Context, options engine.ComposeOptions) error {
			composeArgs = append([]string(nil), options.Args...)
			if options.Stdout != &stdout {
				t.Fatalf("Compose stdout = %#v, want command stdout", options.Stdout)
			}
			return nil
		},
	})
	defer restore()

	if err := cmd.RunFrontendLogsWithWritersForTest(context.Background(), root, config, "", false, &stdout, &stdout); err != nil {
		t.Fatalf("frontend logs: %v", err)
	}
	if got, want := stdout.String(), "Available frontend services: sync, watch-theme, inject\nShowing logs for primary service sync\n"; got != want {
		t.Fatalf("frontend logs output = %q, want %q", got, want)
	}
	if got, want := composeArgs, []string{"logs", "sync"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compose args = %#v, want %#v", got, want)
	}
}

func frontendCommandConfig() engine.Config {
	return engine.Config{
		ProjectName: "sample",
		Framework:   "fake-framework",
		Domain:      "sample.test",
		Stack:       engine.Stack{Features: engine.Features{FrontendSync: true}},
	}
}
