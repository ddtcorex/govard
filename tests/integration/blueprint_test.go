//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestRenderAllFrameworkBlueprints(t *testing.T) {

	frameworks := []string{
		"magento2",
		"magento1",
		"laravel",
		"nextjs",
		"drupal",
		"symfony",
		"shopware",
		"cakephp",
		"wordpress",
		"custom",
	}

	for _, fw := range frameworks {
		t.Run(fw, func(t *testing.T) {
			projectDir := t.TempDir()

			CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

			config := engine.Config{
				ProjectName: "test-" + fw,
				Framework:   fw,
				Domain:      "test-" + fw + ".test",
				Stack: engine.Stack{
					PHPVersion: "8.3",
					WebServer:  "nginx",
					Services: engine.Services{
						WebServer: "nginx",
						Search:    "none",
						Cache:     "none",
						Queue:     "none",
					},
				},
			}

			err := engine.RenderBlueprint(projectDir, config)
			if err != nil {
				t.Fatalf("Failed to render blueprint for %s: %v", fw, err)
			}

			composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
			if _, err := os.Stat(composePath); os.IsNotExist(err) {
				t.Fatalf("Compose file not created for %s", fw)
			}

			content, _ := os.ReadFile(composePath)
			contentStr := string(content)

			if !strings.Contains(contentStr, "services:") {
				t.Errorf("Blueprint for %s missing services section", fw)
			}

			if !strings.Contains(contentStr, "govard-proxy") {
				t.Errorf("Blueprint for %s missing govard-proxy network", fw)
			}
		})
	}
}

func TestRenderBlueprintWithFeatures(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-features",
		Framework:   "magento2",
		Domain:      "test-features.test",
		Stack: engine.Stack{

			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug:  true,
				Cache:   true,
				Varnish: true,
				Search:  true,
			},
			Services: engine.Services{
				WebServer: "nginx",
				DB:        "mariadb",
				Search:    "elasticsearch",
				Cache:     "redis",
				Queue:     "none",
			},
			CacheVersion:  "7.4",
			SearchVersion: "8.11.0",
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	services := LoadComposeServices(t, composePath)

	requiredServices := []string{
		"web",
		"php",
		"php-debug",
		"db",
		"redis",
		"varnish",
		"elasticsearch",
	}

	for _, service := range requiredServices {
		if _, ok := services[service]; !ok {
			t.Errorf("Missing service %s in compose file", service)
		}
	}
}

func TestRenderBlueprintWithCustomWebRoot(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-webroot",
		Framework:   "magento2",
		Domain:      "test-webroot.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			WebRoot:    "custom-public",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, _ := os.ReadFile(composePath)

	if !strings.Contains(string(content), "custom-public") {
		t.Error("Custom web root not applied in rendered blueprint")
	}
}

func TestRenderBlueprintWithVarnish(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-varnish",
		Framework:   "magento2",
		Domain:      "test-varnish.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Varnish: true,
			},
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	services := LoadComposeServices(t, composePath)

	if _, ok := services["varnish"]; !ok {
		t.Error("Varnish service not found in compose file")
	}

	vclPath := filepath.Join(engine.GovardHomeDir(), "varnish", config.ProjectName, "default.vcl")
	if _, err := os.Stat(vclPath); os.IsNotExist(err) {
		t.Error("Varnish VCL file was not created in GovardHomeDir")
	}
}

func TestRenderBlueprintWithXdebug(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-xdebug",
		Framework:   "magento2",
		Domain:      "test-xdebug.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug: true,
			},
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("Failed to read compose file: %v", err)
	}
	contentStr := string(content)
	services := LoadComposeServices(t, composePath)

	if _, ok := services["php-debug"]; !ok {
		t.Error("Xdebug PHP container not found in compose file")
	}

	if !strings.Contains(contentStr, "XDEBUG_MODE: debug") {
		t.Error("XDEBUG_MODE debug configuration not found")
	}
}

func TestRenderBlueprintWithRabbitMQ(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-queue",
		Framework:   "magento2",
		Domain:      "test-queue.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "rabbitmq",
			},
			Features: engine.Features{
				Queue: true,
			},
			QueueVersion: "3.13.7",
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	services := LoadComposeServices(t, composePath)

	if _, ok := services["rabbitmq"]; !ok {
		t.Error("RabbitMQ service not found in compose file")
	}
}

func TestRenderBlueprintWithComposeOverride(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	overrideContent := `
services:
  web:
    environment:
      - CUSTOM_VAR=custom_value
    volumes:
      - ./custom-mount:/custom-mount
`

	overridePath := filepath.Join(projectDir, ".govard", "docker-compose.override.yml")
	if err := os.MkdirAll(filepath.Dir(overridePath), 0755); err != nil {
		t.Fatalf("Failed to create override directory: %v", err)
	}
	if err := os.WriteFile(overridePath, []byte(overrideContent), 0644); err != nil {
		t.Fatalf("Failed to write compose override: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-override",
		Framework:   "magento2",
		Domain:      "test-override.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("Failed to read compose file: %v", err)
	}

	if !strings.Contains(string(content), "CUSTOM_VAR=custom_value") {
		t.Error("Compose override environment variable not merged")
	}

	if !strings.Contains(string(content), "custom-mount") {
		t.Error("Compose override volume not merged")
	}
}

func TestBlueprintNetworkConfiguration(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-network",
		Framework:   "magento2",
		Domain:      "test-network.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("Failed to read compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "govard-proxy:") {
		t.Error("govard-proxy network not defined")
	}

	if !strings.Contains(contentStr, "external: true") {
		t.Error("govard-proxy network should be external")
	}
}

func TestRenderBlueprintWithComposerVersion(t *testing.T) {

	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-composer-ver",
		Framework:   "magento2",
		Domain:      "test-composer-ver.test",
		Stack: engine.Stack{
			PHPVersion:      "8.3",
			WebServer:       "nginx",
			ComposerVersion: "2.2.24",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	versions := []string{"1", "2", "2.2", "2.7.2"}
	for _, v := range versions {
		t.Run("version-"+v, func(t *testing.T) {
			config.Stack.ComposerVersion = v
			err := engine.RenderBlueprint(projectDir, config)
			if err != nil {
				t.Fatalf("Failed to render blueprint for version %s: %v", v, err)
			}

			composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
			content, _ := os.ReadFile(composePath)
			contentStr := string(content)

			if !strings.Contains(contentStr, v) {
				t.Logf("Rendered content for version %s:\n%s", v, contentStr)
				t.Errorf("COMPOSER_VERSION %s not found in rendered blueprint", v)
			}
		})
	}
}
