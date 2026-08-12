package tests

import (
	"os"
	"slices"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"

	"gopkg.in/yaml.v3"
)

type frontendSyncComposeService struct {
	Image       string      `yaml:"image"`
	WorkingDir  string      `yaml:"working_dir"`
	Command     string      `yaml:"command"`
	Volumes     []string    `yaml:"volumes"`
	Networks    []string    `yaml:"networks"`
	Environment interface{} `yaml:"environment"`
	DependsOn   []string    `yaml:"depends_on"`
	Ports       []string    `yaml:"ports"`
	Healthcheck struct {
		Test []string `yaml:"test"`
	} `yaml:"healthcheck"`
}

type frontendSyncCompose struct {
	Services map[string]frontendSyncComposeService `yaml:"services"`
	Volumes  map[string]interface{}                `yaml:"volumes"`
	Networks map[string]struct {
		External bool   `yaml:"external"`
		Name     string `yaml:"name"`
	} `yaml:"networks"`
}

func TestEnvironmentRenderOmitsFrontendServices(t *testing.T) {
	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	writeFrontendSyncTheme(t, root, "Vendor", "Theme", "theme-lock")

	config := frontendSyncRenderConfig()
	if err := engine.RenderBlueprint(root, config); err != nil {
		t.Fatalf("render application blueprint: %v", err)
	}

	compose := readFrontendSyncCompose(t, engine.ComposeFilePath(root, config.ProjectName))
	for name := range compose.Services {
		if name == "sync" || strings.HasPrefix(name, "watch-") {
			t.Fatalf("application compose must omit frontend service %q", name)
		}
	}
	for name := range compose.Volumes {
		if strings.HasPrefix(name, "frontend-sync-") {
			t.Fatalf("application compose must omit frontend volume %q", name)
		}
	}
}

func TestFrontendRenderCreatesHyvaBrowserSyncAndWatchers(t *testing.T) {
	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	writeFrontendSyncTheme(t, root, "Vendor", "Theme", "theme-lock")
	writeFrontendSyncThemeWithoutBrowserSync(t, root, "Vendor", "Additional", "additional-lock")

	config := frontendSyncRenderConfig()
	config.Stack.NodeVersion = "22"
	runtime, composePath, err := magento2.RenderFrontendBlueprint(root, config)
	if err != nil {
		t.Fatalf("render Hyva frontend blueprint: %v", err)
	}
	if runtime.Mode != magento2.FrontendSyncModeHyva {
		t.Fatalf("runtime mode = %q, want Hyva", runtime.Mode)
	}
	if composePath != engine.FrontendComposeFilePath(root, config.ProjectName, config.Profile) {
		t.Fatalf("frontend compose path = %q", composePath)
	}

	compose := readFrontendSyncCompose(t, composePath)
	browserSync, ok := compose.Services["sync"]
	if !ok {
		t.Fatalf("frontend compose must contain BrowserSync, got %v", sortedFrontendSyncMapKeys(compose.Services))
	}
	if browserSync.Image != "node:22-alpine" || !strings.Contains(browserSync.Command, "npm ci") || !strings.Contains(browserSync.Command, "exec npm run browser-sync") {
		t.Fatalf("unexpected BrowserSync service: %#v", browserSync)
	}
	if !slices.Contains(browserSync.Networks, "govard-net") || !slices.Contains(browserSync.Networks, "govard-proxy") {
		t.Fatalf("BrowserSync networks = %#v", browserSync.Networks)
	}
	if len(browserSync.DependsOn) != 0 {
		t.Fatalf("standalone BrowserSync must not declare application dependencies: %#v", browserSync.DependsOn)
	}
	for name, wantNetworkName := range map[string]string{"govard-net": "synthetic-store_govard-net", "govard-proxy": "govard-proxy"} {
		network, ok := compose.Networks[name]
		if !ok || !network.External || network.Name != wantNetworkName {
			t.Fatalf("frontend compose network %q = %#v, want external network named %q", name, network, wantNetworkName)
		}
	}
	for _, watcher := range []string{"watch-vendor-additional", "watch-vendor-theme"} {
		service, ok := compose.Services[watcher]
		if !ok {
			t.Fatalf("frontend compose must contain watcher %q, got %v", watcher, sortedFrontendSyncMapKeys(compose.Services))
		}
		if !strings.Contains(service.Command, "npm ci") || !strings.Contains(service.Command, "exec env NODE_NO_TTY=1 npm run watch") {
			t.Fatalf("unexpected watcher command %q", service.Command)
		}
	}

	injector, ok := compose.Services["inject"]
	if !ok {
		t.Fatalf("frontend compose must contain the BrowserSync HTML injector, got %v", sortedFrontendSyncMapKeys(compose.Services))
	}
	if injector.Image != "node:22-alpine" || !strings.Contains(injector.Command, "frontend-inject.mjs") {
		t.Fatalf("unexpected Hyva injector service: %#v", injector)
	}
	if got := frontendSyncEnvironmentValue(t, injector.Environment, "GOVARD_FRONTEND_INJECT_UPSTREAM"); got != "http://synthetic-store-web-1" {
		t.Fatalf("injector upstream = %q", got)
	}
	if got := frontendSyncEnvironmentValue(t, injector.Environment, "GOVARD_FRONTEND_INJECT_SCRIPT_HTML"); got != `<script src="/browser-sync/browser-sync-client.js"></script>` {
		t.Fatalf("injector script = %q", got)
	}
	if !slices.Contains(injector.Networks, "govard-net") || !slices.Contains(injector.Networks, "govard-proxy") {
		t.Fatalf("injector networks = %#v", injector.Networks)
	}
	if got, want := injector.Healthcheck.Test, []string{"CMD-SHELL", "wget --quiet --spider http://127.0.0.1:3000/__govard_frontend_health"}; !slices.Equal(got, want) {
		t.Fatalf("injector healthcheck = %#v, want %#v", got, want)
	}
}

func TestFrontendRenderCreatesLumaProjectOwnedWatcher(t *testing.T) {
	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	writeFrontendSyncLumaRoot(t, root)

	config := frontendSyncRenderConfig()
	config.Stack.NodeVersion = "22"
	runtime, composePath, err := magento2.RenderFrontendBlueprint(root, config)
	if err != nil {
		t.Fatalf("render Luma frontend blueprint: %v", err)
	}
	if runtime.Mode != magento2.FrontendSyncModeLuma {
		t.Fatalf("runtime mode = %q, want Luma", runtime.Mode)
	}

	compose := readFrontendSyncCompose(t, composePath)
	luma, ok := compose.Services["sync"]
	if !ok {
		t.Fatalf("frontend compose must contain Luma watcher, got %v", sortedFrontendSyncMapKeys(compose.Services))
	}
	if luma.Image != "node:22-alpine" || luma.WorkingDir != "/var/www/html" {
		t.Fatalf("unexpected Luma service: %#v", luma)
	}
	if !strings.Contains(luma.Command, "npm ci") || !strings.Contains(luma.Command, "exec npx grunt watch") {
		t.Fatalf("Luma service must use project-owned npm and Grunt: %q", luma.Command)
	}
	if !slices.Contains(luma.Volumes, "frontend-sync-luma-node-modules-e25b3e26894b0261fafd86d044f10b48e92e8cd8670493effd8207ff149019e5:/var/www/html/node_modules") {
		t.Fatalf("Luma service must mount its private node_modules volume: %#v", luma.Volumes)
	}
	for name, wantNetworkName := range map[string]string{"govard-net": "synthetic-store_govard-net", "govard-proxy": "govard-proxy"} {
		network, ok := compose.Networks[name]
		if !ok || !network.External || network.Name != wantNetworkName {
			t.Fatalf("frontend compose network %q = %#v, want external network named %q", name, network, wantNetworkName)
		}
	}
	if got, want := luma.Healthcheck.Test, []string{"CMD-SHELL", "wget --quiet --spider http://127.0.0.1:35729"}; !slices.Equal(got, want) {
		t.Fatalf("Luma healthcheck = %#v, want %#v", got, want)
	}

	injector, ok := compose.Services["inject"]
	if !ok {
		t.Fatalf("frontend compose must contain the Luma HTML injector, got %v", sortedFrontendSyncMapKeys(compose.Services))
	}
	if injector.Image != "node:22-alpine" || !strings.Contains(injector.Command, "frontend-inject.mjs") {
		t.Fatalf("unexpected Luma injector service: %#v", injector)
	}
	if got := frontendSyncEnvironmentValue(t, injector.Environment, "GOVARD_FRONTEND_INJECT_UPSTREAM"); got != "http://synthetic-store-web-1" {
		t.Fatalf("injector upstream = %q", got)
	}
	if got := frontendSyncEnvironmentValue(t, injector.Environment, "GOVARD_FRONTEND_INJECT_SCRIPT_HTML"); got != `<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>` {
		t.Fatalf("injector script = %q", got)
	}
	if !slices.Contains(injector.Networks, "govard-net") || !slices.Contains(injector.Networks, "govard-proxy") {
		t.Fatalf("injector networks = %#v", injector.Networks)
	}
	if got, want := injector.Healthcheck.Test, []string{"CMD-SHELL", "wget --quiet --spider http://127.0.0.1:3000/__govard_frontend_health"}; !slices.Equal(got, want) {
		t.Fatalf("injector healthcheck = %#v, want %#v", got, want)
	}
}

func frontendSyncEnvironmentValue(t *testing.T, environment interface{}, key string) string {
	t.Helper()
	switch values := environment.(type) {
	case map[string]interface{}:
		value, _ := values[key].(string)
		return value
	case map[string]string:
		return values[key]
	default:
		t.Fatalf("environment = %#v, want map form", environment)
		return ""
	}
}

func readFrontendSyncCompose(t *testing.T, composePath string) frontendSyncCompose {
	t.Helper()
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read rendered compose: %v", err)
	}
	var compose frontendSyncCompose
	if err := yaml.Unmarshal(content, &compose); err != nil {
		t.Fatalf("parse rendered compose: %v", err)
	}
	return compose
}

func sortedFrontendSyncMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func frontendSyncRenderConfig() engine.Config {
	return engine.Config{
		ProjectName: "synthetic-store",
		Framework:   "magento2",
		Domain:      "synthetic-store.test",
		Stack: engine.Stack{
			Features: engine.Features{FrontendSync: true},
			Services: engine.Services{WebServer: "nginx", Search: "none", Cache: "none", Queue: "none"},
		},
	}
}
