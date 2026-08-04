package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

// This catches the consolidation regression where a checkout-local framework
// blueprint was ignored because only the compiled-in union filesystem was
// considered after assets moved out of internal/blueprints/files.
func TestRenderBlueprintUsesCheckoutFrameworkAssetOverride(t *testing.T) {
	checkout := t.TempDir()
	projectDir := filepath.Join(checkout, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// This directory marks checkout as a source-tree layout. Its framework
	// asset overlays the embedded Laravel template, while all other assets
	// continue to come from the embedded base filesystem.
	sharedDir := filepath.Join(checkout, "internal", "blueprints", "files")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("create shared blueprints directory: %v", err)
	}
	frameworkBlueprintDir := filepath.Join(checkout, "internal", "frameworks", "laravel", "blueprint")
	if err := os.MkdirAll(frameworkBlueprintDir, 0o755); err != nil {
		t.Fatalf("create Laravel blueprint directory: %v", err)
	}
	const marker = "# checkout-laravel-override"
	if err := os.WriteFile(filepath.Join(frameworkBlueprintDir, "laravel.conf"), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatalf("write Laravel override: %v", err)
	}

	setTestGovardHome(t, checkout)
	config := engine.Config{
		ProjectName: "checkout-override",
		Framework:   "laravel",
		Domain:      "checkout-override.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				DB:        "mariadb",
			},
		},
	}
	if err := engine.RenderBlueprint(projectDir, config); err != nil {
		t.Fatalf("render blueprint: %v", err)
	}

	nginxPath := filepath.Join(engine.GovardHomeDir(), "nginx", config.ProjectName, "default.conf")
	nginx, err := os.ReadFile(nginxPath)
	if err != nil {
		t.Fatalf("read rendered nginx config: %v", err)
	}
	if !strings.Contains(string(nginx), marker) {
		t.Fatalf("expected checkout framework blueprint marker in rendered nginx config, got:\n%s", nginx)
	}
}
