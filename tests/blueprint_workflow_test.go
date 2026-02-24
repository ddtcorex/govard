package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"govard/internal/engine"
)

func TestFullSetupLogic(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "full-setup-*")
	defer os.RemoveAll(tempDir)

	projectName := filepath.Base(tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "blueprints")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: projectName,
		Recipe:      "magento2",
		Domain:      projectName + ".test",
		Stack: engine.Stack{
			PHPVersion: "8.1",
			WebServer:  "nginx",
			Features: engine.Features{
				Varnish: true,
			},
		},
	}

	data, _ := yaml.Marshal(&config)
	os.WriteFile(filepath.Join(tempDir, ".govard.yml"), data, 0644)

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	renderPath := engine.ComposeFilePath(tempDir, config.ProjectName)
	rendered, _ := os.ReadFile(renderPath)

	if !strings.Contains(string(rendered), "govard-proxy") {
		t.Error("Rendered compose file missing govard-proxy network")
	}

	if !strings.Contains(string(rendered), "external: true") {
		t.Error("govard-proxy network should be marked as external")
	}
}
