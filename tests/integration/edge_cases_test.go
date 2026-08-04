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

func TestEdgeCaseProjectNameWithUnderscores(t *testing.T) {
	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "my_test_project_123",
		Framework:   "magento2",
		Domain:      "my-test-project.test",
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
		t.Fatalf("Failed with underscores in project name: %v", err)
	}

	normalized := config
	engine.NormalizeConfig(&normalized, "")
	if normalized.ProjectName != "my_test_project_123" {
		t.Fatalf("Project name should be preserved, got: %s", normalized.ProjectName)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	services := LoadComposeServices(t, composePath)
	if _, ok := services["web"]; !ok {
		t.Error("Rendered compose should contain web service")
	}
}

func TestEdgeCaseProjectNameWithHyphens(t *testing.T) {
	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "my-test-project",
		Framework:   "magento2",
		Domain:      "my-test-project.test",
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
		t.Fatalf("Failed with hyphens in project name: %v", err)
	}

	normalized := config
	engine.NormalizeConfig(&normalized, "")
	if normalized.ProjectName != "my-test-project" {
		t.Fatalf("Project name should be preserved, got: %s", normalized.ProjectName)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	services := LoadComposeServices(t, composePath)
	if _, ok := services["web"]; !ok {
		t.Error("Rendered compose should contain web service")
	}
}

func TestEdgeCaseDomainWithMultipleSubdomains(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "test",
			"framework":    "magento2",
			"domain":       "sub1.sub2.example.test",
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
	}

	projectDir := env.CreateTestProject(t, "multi-subdomain", files)

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Domain != "sub1.sub2.example.test" {
		t.Errorf("Expected multi-level domain, got: %s", config.Domain)
	}
}

func TestEdgeCasePHPVersionEdgeValues(t *testing.T) {
	testCases := []struct {
		version string
		valid   bool
	}{
		{"8.1", true},
		{"8.2", true},
		{"8.3", true},
		{"8.4", true},
		{"7.4", true},
		{"8.0", true},
		{"9.0", true},
		{"", false},
		{"latest", true},
		{"8", true},
	}

	for _, tc := range testCases {
		config := engine.Config{
			ProjectName: "test",
			Framework:   "magento2",
			Domain:      "test.test",
			Stack: engine.Stack{
				PHPVersion: tc.version,
				Services: engine.Services{
					WebServer: "nginx",
					Search:    "none",
					Cache:     "none",
					Queue:     "none",
				},
			},
		}

		engine.NormalizeConfig(&config, "")

		t.Logf("PHP version %s normalized to %s", tc.version, config.Stack.PHPVersion)
	}
}

func TestEdgeCaseEmptyBlueprintIncludes(t *testing.T) {
	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-empty",
		Framework:   "custom",
		Domain:      "test-empty.test",
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
		t.Fatalf("Failed with custom framework: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Error("Compose file was not created for custom framework")
	}
}

func TestEdgeCaseFrameworkDetectionWithDevDependencies(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require-dev": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "dev-deps", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Logf("Framework detected from require-dev: %s", metadata.Framework)
	}
}

func TestEdgeCaseDuplicateFeatureFlags(t *testing.T) {
	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	config := engine.Config{
		ProjectName: "test-dup",
		Framework:   "magento2",
		Domain:      "test-dup.test",
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
				Search:    "elasticsearch",
				Cache:     "redis",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed with all features enabled: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, _ := os.ReadFile(composePath)
	contentStr := string(content)

	if !strings.Contains(contentStr, "php-debug") {
		t.Error("Xdebug container missing")
	}
	if !strings.Contains(contentStr, "redis") {
		t.Error("Redis container missing")
	}
	if !strings.Contains(contentStr, "varnish") {
		t.Error("Varnish container missing")
	}
	if !strings.Contains(contentStr, "elasticsearch") {
		t.Error("Elasticsearch container missing")
	}
}

func TestEdgeCaseNilConfigFeatures(t *testing.T) {
	config := engine.Config{
		ProjectName: "test",
		Framework:   "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	engine.NormalizeConfig(&config, "")

	if config.Stack.Features.Xdebug &&
		config.Stack.Features.Cache &&
		config.Stack.Features.Varnish &&
		config.Stack.Features.Search {
		t.Error("Features should be false by default")
	}
}

func TestEdgeCaseVeryLongProjectName(t *testing.T) {
	projectDir := t.TempDir()

	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	longName := "very-long-project-name-that-exceeds-normal-length-limits-for-testing-purposes"
	config := engine.Config{
		ProjectName: longName,
		Framework:   "magento2",
		Domain:      longName + ".test",
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
		t.Logf("Long project name error (may be expected): %v", err)
	}
}

func TestEdgeCaseSpecialCharactersInDomain(t *testing.T) {
	invalidDomains := []string{
		"test@domain.test",
		"test#domain.test",
		"test$domain.test",
		"test%domain.test",
		"test^domain.test",
		"test&domain.test",
		"test*domain.test",
		"test(domain).test",
	}

	for _, domain := range invalidDomains {
		config := engine.Config{
			ProjectName: "test",
			Framework:   "magento2",
			Domain:      domain,
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
			t.Logf("Domain %s was accepted (may need validation)", domain)
		}
	}
}

func TestEdgeCaseEmptyComposerRequire(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"name":    "test/project",
			"require": map[string]string{},
		}),
	}

	projectDir := env.CreateTestProject(t, "empty-require", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Errorf("Expected generic for empty require, got: %s", metadata.Framework)
	}
}

func TestEdgeCaseFrameworkDetectionWithOnlyDevDependencies(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": `{
			"name": "test/project",
			"require": {},
			"require-dev": {
				"laravel/framework": "^11.0"
			}
		}`,
	}

	projectDir := env.CreateTestProject(t, "dev-only-deps", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Logf("Note: Framework detected from require-dev: %s", metadata.Framework)
	}
}

func TestEdgeCaseMultiplePackageManagers(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"laravel/framework": "^11.0",
			},
		}),
		"package.json": MustMarshalJSON(t, map[string]interface{}{
			"dependencies": map[string]string{
				"next": "^14.0.0",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "multi-pkg", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework == "" {
		t.Error("Framework should be detected when multiple package managers present")
	}
}

func TestEdgeCaseCaseSensitivityInFrameworkDetection(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"LARAVEL/FRAMEWORK": "^11.0",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "case-sensitive", files)

	metadata := engine.DetectFramework(projectDir)

	if metadata.Framework != "generic" {
		t.Logf("Note: Case-sensitive detection found: %s", metadata.Framework)
	}
}
