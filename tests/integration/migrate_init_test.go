//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWithMigrateFrom(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := t.TempDir()

	// Setup DDEV files
	ddevDir := filepath.Join(projectDir, ".ddev")
	if err := os.MkdirAll(ddevDir, 0755); err != nil {
		t.Fatal(err)
	}
	configYaml := `name: migrate-test
type: laravel
php_version: "8.3"
`
	if err := os.WriteFile(filepath.Join(ddevDir, "config.yaml"), []byte(configYaml), 0644); err != nil {
		t.Fatal(err)
	}

	result := env.RunGovard(t, projectDir, "init", "--migrate-from", "ddev")
	result.AssertSuccess(t)

	// Verify .govard.yml exists and has migrated values
	configPath := filepath.Join(projectDir, ".govard.yml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected .govard.yml to be created: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "project_name: migrate-test") {
		t.Errorf("expected project_name: migrate-test in config, got:\n%s", content)
	}
	if !strings.Contains(content, "recipe: laravel") {
		t.Errorf("expected recipe: laravel in config, got:\n%s", content)
	}
	if !strings.Contains(content, "php_version: \"8.3\"") {
		t.Errorf("expected php_version: \"8.3\" in config, got:\n%s", content)
	}
}
