//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"gopkg.in/yaml.v3"
)

type frontendSyncIntegrationCompose struct {
	Services map[string]struct {
		Command string `yaml:"command"`
	} `yaml:"services"`
}

func TestFrontendSyncRendersDedicatedHyvaCompose(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))
	writeFrontendSyncIntegrationTheme(t, projectDir, "Vendor", "Theme")
	writeFrontendSyncIntegrationThemeWithoutBrowserSync(t, projectDir, "Vendor", "Additional")

	config := engine.Config{
		ProjectName: "synthetic-store",
		Framework:   "magento2",
		Domain:      "synthetic-store.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features:   engine.Features{FrontendSync: true},
			Services:   engine.Services{WebServer: "nginx", Search: "none", Cache: "none", Queue: "none"},
		},
	}

	if err := engine.RenderBlueprint(projectDir, config); err != nil {
		t.Fatalf("render application compose: %v", err)
	}
	render, ok := frameworks.FrontendSyncRenderer(config.Framework)
	if !ok {
		t.Fatalf("frontend renderer is not registered for %q", config.Framework)
	}
	_, frontendComposePath, err := render(projectDir, config)
	if err != nil {
		t.Fatalf("render frontend compose: %v", err)
	}
	if _, err := exec.LookPath("docker"); err == nil {
		validate := exec.Command("docker", "compose", "-f", frontendComposePath, "config")
		if output, err := validate.CombinedOutput(); err != nil {
			t.Fatalf("validate standalone frontend compose with docker compose config: %v\n%s", err, output)
		}
	}

	applicationSource, err := os.ReadFile(engine.ComposeFilePath(projectDir, config.ProjectName))
	if err != nil {
		t.Fatalf("read application compose: %v", err)
	}
	var application frontendSyncIntegrationCompose
	if err := yaml.Unmarshal(applicationSource, &application); err != nil {
		t.Fatalf("decode application compose: %v", err)
	}
	for name := range application.Services {
		if name == "sync" || strings.HasPrefix(name, "watch-") {
			t.Fatalf("application compose contains frontend service %q:\n%s", name, applicationSource)
		}
	}

	frontendSource, err := os.ReadFile(frontendComposePath)
	if err != nil {
		t.Fatalf("read frontend compose: %v", err)
	}
	var frontend frontendSyncIntegrationCompose
	if err := yaml.Unmarshal(frontendSource, &frontend); err != nil {
		t.Fatalf("decode frontend compose: %v", err)
	}
	for name := range map[string]struct{}{"sync": {}, "watch-vendor-theme": {}, "watch-vendor-additional": {}} {
		service, ok := frontend.Services[name]
		if !ok {
			t.Fatalf("frontend compose missing %q: %v", name, frontend.Services)
		}
		if !strings.Contains(service.Command, "npm ci") {
			t.Fatalf("%s does not install project dependencies: %q", name, service.Command)
		}
	}
}

func writeFrontendSyncIntegrationTheme(t *testing.T, projectDir, vendor, theme string) {
	t.Helper()
	tailwindDir := filepath.Join(projectDir, "app", "design", "frontend", vendor, theme, "web", "tailwind")
	if err := os.MkdirAll(tailwindDir, 0o755); err != nil {
		t.Fatalf("create synthetic Tailwind fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tailwindDir, "package.json"), []byte(`{"scripts":{"watch":"tailwindcss --watch","browser-sync":"browser-sync start --config browser-sync.config.js"}}`), 0o644); err != nil {
		t.Fatalf("write synthetic package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tailwindDir, "package-lock.json"), []byte(`{"lockfileVersion":3}`), 0o644); err != nil {
		t.Fatalf("write synthetic package-lock.json: %v", err)
	}
}

func writeFrontendSyncIntegrationThemeWithoutBrowserSync(t *testing.T, projectDir, vendor, theme string) {
	t.Helper()
	writeFrontendSyncIntegrationTheme(t, projectDir, vendor, theme)
	packagePath := filepath.Join(projectDir, "app", "design", "frontend", vendor, theme, "web", "tailwind", "package.json")
	if err := os.WriteFile(packagePath, []byte(`{"scripts":{"watch":"tailwindcss --watch"}}`), 0o644); err != nil {
		t.Fatalf("write synthetic watcher package.json: %v", err)
	}
}
