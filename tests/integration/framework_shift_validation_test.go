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

func TestFrameworkProfileShiftDetection(t *testing.T) {
	frameworks := []string{"magento2", "magento1", "wordpress", "symfony", "laravel"}

	for _, fw := range frameworks {
		t.Run(fw, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "govard-test-shift-"+fw+"-*")
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
						ProjectName:      "test-" + fw,
						PHPVersion:       "8.1",
						Profile:          "default",
						Framework:        fw,
						FrameworkVersion: "1.0.0",
					},
				},
			}
			regData, _ := json.Marshal(registry)
			_ = os.WriteFile(registryPath, regData, 0644)

			// Case: PHP Version shift
			config := engine.Config{
				ProjectName:      "test-" + fw,
				Framework:        fw,
				FrameworkVersion: "1.0.0",
				Profile:          "default",
			}
			config.Stack.PHPVersion = "8.2"

			info := magento2.DetectProfileShiftForTest(config)
			if !info.Shifted {
				t.Errorf("[%s] Expected shift to be detected for PHP version change", fw)
			}
			if info.PreviousPHP != "8.1" || info.CurrentPHP != "8.2" {
				t.Errorf("[%s] Incorrect PHP versions: prev=%s, curr=%s", fw, info.PreviousPHP, info.CurrentPHP)
			}

			// Case: Profile shift
			config.Stack.PHPVersion = "8.1"
			config.Profile = "staging"

			info = magento2.DetectProfileShiftForTest(config)
			if !info.Shifted {
				t.Errorf("[%s] Expected shift to be detected for Profile change", fw)
			}
			if info.PreviousProfile != "default" || info.CurrentProfile != "staging" {
				t.Errorf("[%s] Incorrect profiles: prev=%s, curr=%s", fw, info.PreviousProfile, info.CurrentProfile)
			}
		})
	}
}
