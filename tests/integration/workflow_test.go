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

// TestEndToEndMagento2Workflow tests the complete Magento 2 workflow
func TestEndToEndMagento2Workflow(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateMagento2Project(t, "magento2-integration")
	defer env.CleanupProject(t, "magento2-integration")

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "magento2-test",
		Framework:   "magento2",
		Domain:      "magento2-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.1",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug: false,
				Cache:  false,
			},
		},
	}
	CreateGovardConfig(t, projectDir, config)

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatal("Compose file was not created")
	}

	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("Failed to read compose file: %v", err)
	}
	services := LoadComposeServices(t, composePath)
	if !strings.Contains(string(content), "govard-proxy") {
		t.Error("Compose file missing govard-proxy network")
	}
	if _, ok := services["web"]; !ok {
		t.Error("Compose file missing web service")
	}
}

// TestEndToEndLaravelWorkflow tests the complete Laravel workflow
func TestEndToEndLaravelWorkflow(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateLaravelProject(t, "laravel-integration")
	defer env.CleanupProject(t, "laravel-integration")

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "laravel-test",
		Framework:   "laravel",
		Domain:      "laravel-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug: false,
			},
		},
	}
	CreateGovardConfig(t, projectDir, config)

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatal("Compose file was not created")
	}

	services := LoadComposeServices(t, composePath)
	if _, ok := services["web"]; !ok {
		t.Error("Compose file missing web service")
	}
}

// TestEndToEndNextJSWorkflow tests the complete Next.js workflow
func TestEndToEndNextJSWorkflow(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateNextJSProject(t, "nextjs-integration")
	defer env.CleanupProject(t, "nextjs-integration")

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "nextjs-test",
		Framework:   "nextjs",
		Domain:      "nextjs-test.test",
		Stack: engine.Stack{
			WebServer: "nginx",
			Features: engine.Features{
				Xdebug: false,
			},
		},
	}
	CreateGovardConfig(t, projectDir, config)

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatal("Compose file was not created")
	}
}

// TestBlueprintRenderingWithAllFeatures tests blueprint with all features enabled
func TestBlueprintRenderingWithAllFeatures(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateMagento2Project(t, "magento2-full")
	defer env.CleanupProject(t, "magento2-full")

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "magento2-full",
		Framework:   "magento2",
		Domain:      "magento2-full.test",
		Stack: engine.Stack{
			PHPVersion: "8.1",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug:  true,
				Cache:   true,
				Varnish: true,
				Search:  true,
				Queue:   true,
			},
			Services: engine.Services{
				Cache:  "redis",
				Search: "elasticsearch",
				Queue:  "rabbitmq",
			},
			CacheVersion:  "7.4",
			SearchVersion: "8.11.0",
			QueueVersion:  "3.13.7",
		},
	}
	CreateGovardConfig(t, projectDir, config)

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

	requiredServices := []string{
		"web",
		"php",
		"php-debug",
		"redis",
		"varnish",
		"elasticsearch",
		"rabbitmq",
	}

	for _, service := range requiredServices {
		if _, ok := services[service]; !ok {
			t.Errorf("Missing service %s in compose file", service)
		}
	}

	if !strings.Contains(contentStr, "XDEBUG_MODE: debug") {
		t.Error("Missing Xdebug debug mode configuration")
	}

	varnishVclPath := filepath.Join(engine.GovardHomeDir(), "varnish", config.ProjectName, "default.vcl")
	if _, err := os.Stat(varnishVclPath); os.IsNotExist(err) {
		t.Error("Varnish VCL file was not created in GovardHomeDir")
	}
}
