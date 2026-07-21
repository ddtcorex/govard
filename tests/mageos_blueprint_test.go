package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"govard/internal/engine"
)

func TestMageOSRequiredRuntimeImageIsPHPMagento2(t *testing.T) {
	config := engine.Config{
		ProjectName: "mageos-test",
		Framework:   "mageos",
		Domain:      "mageos-test.test",
	}
	engine.NormalizeConfig(&config, t.TempDir())

	images := engine.RequiredRuntimeImages(config, "")
	found := false
	for _, img := range images {
		if img == "" {
			continue
		}
		// The php image tag always contains "php-magento2:" for magento2
		// and, after this task, for mageos too.
		if containsSubstring(img, "php-magento2:") {
			found = true
		}
		if containsSubstring(img, "php-mageos") {
			t.Errorf("expected no separate php-mageos image family, got %q", img)
		}
	}
	if !found {
		t.Errorf("expected a php-magento2:* image in %v", images)
	}
}

func TestMageOSBlueprintRendersLikeMagento2(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	tempDir := t.TempDir()
	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("failed to copy blueprints: %v", err)
	}

	fakeHome := filepath.Join(tempDir, "fake-home")
	if err := os.MkdirAll(fakeHome, 0o755); err != nil {
		t.Fatalf("failed to create fake home dir: %v", err)
	}
	t.Setenv("HOME", fakeHome)
	t.Setenv("SSH_AUTH_SOCK", "")

	projectName := "mageos-blueprint-test"
	config := engine.Config{
		ProjectName: projectName,
		Framework:   "mageos",
		Domain:      projectName + ".test",
		Stack: engine.Stack{
			UserID:  1000,
			GroupID: 1000,
			Features: engine.Features{
				Varnish: true,
			},
		},
	}
	engine.NormalizeConfig(&config, tempDir)

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("RenderBlueprint failed for mageos: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, projectName)
	composeContent, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read rendered compose file: %v", err)
	}
	if !containsSubstring(string(composeContent), "php-magento2:") {
		t.Error("expected mageos compose file to use the php-magento2 image, like magento2")
	}

	nginxPath := filepath.Join(engine.GovardHomeDir(), "nginx", projectName, "default.conf")
	if _, err := os.ReadFile(nginxPath); err != nil {
		t.Errorf("expected an nginx config to be rendered for mageos (reusing magento2.conf), got error: %v", err)
	}
}
