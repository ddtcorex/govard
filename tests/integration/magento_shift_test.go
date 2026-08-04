//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
	"os"
	"path/filepath"
	"testing"
)

func TestMagentoProfileShiftDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "govard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Mock Registry
	registryPath := filepath.Join(tmpDir, "projects.json")
	if err := os.Setenv("GOVARD_PROJECT_REGISTRY_PATH", registryPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("GOVARD_PROJECT_REGISTRY_PATH") }()

	registry := struct {
		Version  int                           `json:"version"`
		Projects []engine.ProjectRegistryEntry `json:"projects"`
	}{
		Version: 1,
		Projects: []engine.ProjectRegistryEntry{
			{
				Path:             tmpDir,
				ProjectName:      "test-project",
				PHPVersion:       "8.2",
				Profile:          "default",
				FrameworkVersion: "2.4.6-p3",
			},
		},
	}
	regData, _ := json.Marshal(registry)
	_ = os.WriteFile(registryPath, regData, 0644)

	// Case 1: No change
	config := engine.Config{}
	config.Stack.PHPVersion = "8.2"
	config.Profile = "default"
	config.FrameworkVersion = "2.4.6-p3"

	shifted, reason := magento2.CheckProfileShiftCleanupForTest(config)
	if shifted {
		t.Errorf("Expected no shift, got %v: %s", shifted, reason)
	}

	// Case 2: PHP Version change
	config.Stack.PHPVersion = "8.4"
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if !shifted || reason != "PHP version changed: 8.2 -> 8.4" {
		t.Errorf("Expected PHP shift, got shifted=%v, reason=%s", shifted, reason)
	}

	// Case 3: Profile change
	config.Stack.PHPVersion = "8.2"
	config.Profile = "upgrade"
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if !shifted || reason != "Profile changed: \"default\" -> \"upgrade\"" {
		t.Errorf("Expected Profile shift, got shifted=%v, reason=%s", shifted, reason)
	}

	// Case 4: Framework Version change
	config.Profile = "default"
	config.FrameworkVersion = "2.4.8-p4"
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if !shifted || reason != "Version changed: 2.4.6-p3 -> 2.4.8-p4" {
		t.Errorf("Expected Version shift, got shifted=%v, reason=%s", shifted, reason)
	}

	// Case 5: Empty current PHP should NOT trigger shift (user wants profile default)
	// This is the fix: empty PHP version means "use profile default", not "change"
	config.Stack.PHPVersion = ""
	config.FrameworkVersion = "2.4.6-p3"
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if shifted {
		t.Errorf("Expected no shift when current PHP is empty, got shifted=%v: %s", shifted, reason)
	}

	// Case 6: Empty current profile should NOT trigger shift
	config.Stack.PHPVersion = "8.2"
	config.Profile = ""
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if shifted {
		t.Errorf("Expected no shift when current profile is empty, got shifted=%v: %s", shifted, reason)
	}

	// Case 7: Empty current version should NOT trigger shift
	config.Profile = "default"
	config.FrameworkVersion = ""
	shifted, reason = magento2.CheckProfileShiftCleanupForTest(config)
	if shifted {
		t.Errorf("Expected no shift when current version is empty, got shifted=%v: %s", shifted, reason)
	}
}

func TestDetectMagentoProfileShiftInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "govard-test-shift-info-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	registryPath := filepath.Join(tmpDir, "projects.json")
	if err := os.Setenv("GOVARD_PROJECT_REGISTRY_PATH", registryPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("GOVARD_PROJECT_REGISTRY_PATH") }()

	registry := struct {
		Version  int                           `json:"version"`
		Projects []engine.ProjectRegistryEntry `json:"projects"`
	}{
		Version: 1,
		Projects: []engine.ProjectRegistryEntry{
			{
				Path:             tmpDir,
				ProjectName:      "test-project",
				PHPVersion:       "8.2",
				Profile:          "default",
				FrameworkVersion: "2.4.6-p3",
			},
		},
	}
	regData, _ := json.Marshal(registry)
	_ = os.WriteFile(registryPath, regData, 0644)

	t.Run("NoShiftReturnsEmpty", func(t *testing.T) {
		config := engine.Config{}
		config.Stack.PHPVersion = "8.2"
		config.Profile = "default"
		config.FrameworkVersion = "2.4.6-p3"

		info := magento2.DetectProfileShiftForTest(config)
		if info.Shifted {
			t.Errorf("Expected no shift, got Shifted=true, Reason=%s", info.Reason)
		}
	})

	t.Run("VersionShiftPopulatesFields", func(t *testing.T) {
		config := engine.Config{}
		config.Stack.PHPVersion = "8.4"
		config.Profile = "default"
		config.FrameworkVersion = "2.4.8-p4"

		info := magento2.DetectProfileShiftForTest(config)
		if !info.Shifted {
			t.Fatal("Expected shift to be detected")
		}
		if info.PreviousPHP != "8.2" {
			t.Errorf("Expected PreviousPHP=8.2, got %s", info.PreviousPHP)
		}
		if info.CurrentPHP != "8.4" {
			t.Errorf("Expected CurrentPHP=8.4, got %s", info.CurrentPHP)
		}
		if info.PreviousVersion != "2.4.6-p3" {
			t.Errorf("Expected PreviousVersion=2.4.6-p3, got %s", info.PreviousVersion)
		}
		if info.CurrentVersion != "2.4.8-p4" {
			t.Errorf("Expected CurrentVersion=2.4.8-p4, got %s", info.CurrentVersion)
		}
		if info.IsInitial {
			t.Error("Expected IsInitial=false")
		}
	})

	t.Run("NoPreviousInfoDoesNotTriggerShift", func(t *testing.T) {
		// Use a different dir with no registry entry
		noRegDir, err := os.MkdirTemp("", "govard-test-initial-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(noRegDir)
		if err := os.Chdir(noRegDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(tmpDir) }()

		config := engine.Config{}
		config.Stack.PHPVersion = "8.4"
		config.FrameworkVersion = "2.4.8-p4"

		info := magento2.DetectProfileShiftForTest(config)
		if info.Shifted {
			t.Fatal("Expected NO shift when there's no previous info (user runs `govard config auto` manually)")
		}
	})
}
