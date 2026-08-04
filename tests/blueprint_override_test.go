package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/blueprints"
	"govard/internal/engine"
)

func TestRenderBlueprintMergesProjectComposeOverride(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("copy blueprints: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(tempDir, ".govard"), 0755); err != nil {
		t.Fatalf("create .govard dir: %v", err)
	}

	override := `services:
  php:
    environment:
      GOVARD_OVERRIDE_MARKER: "1"
  helper:
    image: alpine:3.20
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard", "docker-compose.override.yml"), []byte(override), 0644); err != nil {
		t.Fatalf("write override file: %v", err)
	}

	config := engine.Config{
		ProjectName: "demo",
		Framework:   "laravel",
		Domain:      "demo.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",

			DBVersion: "11.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "opensearch",
				Cache:     "redis",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("read generated compose file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "GOVARD_OVERRIDE_MARKER: \"1\"") {
		t.Fatal("expected php service override marker in generated compose")
	}
	if !strings.Contains(text, "helper:") {
		t.Fatal("expected custom helper service in generated compose")
	}
}
