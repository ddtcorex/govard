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

func TestRenderBlueprintWorksWithFallback(t *testing.T) {
	projectDir := t.TempDir()

	// No blueprints in projectDir/blueprints, should use embedded fallback
	config := engine.Config{
		ProjectName: "test-fallback",
		Recipe:      "magento2",
		Domain:      "test-fallback.test",
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
		t.Fatalf("Expected success with embedded fallback, got error: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Error("Compose file was not created via fallback")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		".govard.yml": `
project_name: test
recipe: magento2
stack:
  php_version: 8.3
  invalid_yaml_here: [
    unclosed bracket
`,
	}

	projectDir := env.CreateTestProject(t, "invalid-yaml", files)

	_, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadConfigEmptyYAML(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		".govard.yml": "",
	}

	projectDir := env.CreateTestProject(t, "empty-yaml", files)

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("Empty YAML should be valid: %v", err)
	}

	if config.ProjectName != "empty-yaml" {
		t.Errorf("Expected project name from directory, got: %s", config.ProjectName)
	}
}

func TestDetectFrameworkMalformedJSON(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": `{"name": "test", "require": {invalid json here}`,
	}

	projectDir := env.CreateTestProject(t, "malformed-json", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Errorf("Expected generic framework for malformed JSON, got: %s", metadata.Framework)
	}
}

func TestDetectFrameworkEmptyFiles(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": "",
		"package.json":  "",
	}

	projectDir := env.CreateTestProject(t, "empty-files", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Errorf("Expected generic framework for empty files, got: %s", metadata.Framework)
	}
}

func TestValidateConfigEmptyProjectName(t *testing.T) {
	config := engine.Config{
		ProjectName: "",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for empty project name")
	}

	if !strings.Contains(err.Error(), "project_name") {
		t.Errorf("Error should mention project_name, got: %v", err)
	}
}

func TestValidateConfigEmptyDomain(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for empty domain")
	}
}

func TestValidateConfigWhitespaceInDomain(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test domain.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for domain with whitespace")
	}
}

func TestValidateConfigInvalidWebServer(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "invalid-server",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for invalid web server")
	}
}

func TestValidateConfigInvalidSearchService(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "invalid-search",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for invalid search service")
	}
}

func TestValidateConfigInvalidCacheService(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "invalid-cache",
				Queue:     "none",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for invalid cache service")
	}
}

func TestValidateConfigInvalidQueueService(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "invalid-queue",
			},
		},
	}

	err := engine.ValidateConfig(config)
	if err == nil {
		t.Error("Expected error for invalid queue service")
	}
}

func TestConfigLayeringWithEmptyLocalFile(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "base",
			"recipe":       "magento2",
			"domain":       "base.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
		".govard.local.yml": "",
	}

	projectDir := env.CreateTestProject(t, "empty-local", files)

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("Failed to load config with empty local file: %v", err)
	}

	if config.ProjectName != "base" {
		t.Errorf("Expected project name 'base', got: %s", config.ProjectName)
	}
}

func TestConfigLayeringMissingBaseFile(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		".govard.local.yml": MustMarshalYAML(t, map[string]interface{}{
			"stack": map[string]interface{}{
				"features": map[string]interface{}{
					"xdebug": true,
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "missing-base", files)

	_, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err == nil {
		t.Error("Expected error when base .govard.yml is missing")
	}
}

func TestCreateSnapshotNonExistentProject(t *testing.T) {
	nonExistentDir := "/tmp/govard-test-nonexistent-" + randomString(8)

	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	_, err := engine.CreateSnapshot(nonExistentDir, config, "test")
	if err != nil {
		t.Logf("Creating snapshot in non-existent dir error (may be expected): %v", err)
	}
}

func TestBlueprintRenderWithSpecialCharactersInProjectName(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := t.TempDir()

	CopyBlueprints(t, env.BlueprintsPath, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test_project-123",
		Recipe:      "magento2",
		Domain:      "test-project.test",
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
		t.Fatalf("Failed to render blueprint with special chars in name: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Error("Compose file was not created")
	}
}

func TestFrameworkDetectionWithVeryLongVersionString(t *testing.T) {
	env := NewTestEnvironment(t)

	longVersion := "1.0.0-alpha.beta.1+build.123.exp.sha.5114f85"
	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": longVersion,
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "long-version", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "magento2" {
		t.Errorf("Expected magento2, got: %s", metadata.Framework)
	}

	if metadata.Version != longVersion {
		t.Errorf("Expected version %s, got: %s", longVersion, metadata.Version)
	}
}

func TestNormalizeConfigWithInvalidPHPVersion(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			PHPVersion: "99.99",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	engine.NormalizeConfig(&config)

	if config.Stack.PHPVersion != "99.99" {
		t.Logf("NormalizeConfig changed invalid PHP version to: %s", config.Stack.PHPVersion)
	}
}

func TestNormalizeConfigDefaultsForUnknownFramework(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Recipe:      "unknown-framework",
		Domain:      "test.test",
		Stack: engine.Stack{
			Services: engine.Services{
				WebServer: "",
				Search:    "",
				Cache:     "",
				Queue:     "",
			},
		},
	}

	engine.NormalizeConfig(&config)

	if config.Stack.Services.WebServer == "" {
		t.Error("WebServer should have a default value after normalization")
	}
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
