package tests

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestMaybeRunMagentoComposerInstallSkipsWhenVendorSatisfiesLock(t *testing.T) {
	projectRoot := t.TempDir()
	chdirForTest(t, projectRoot)

	if err := os.WriteFile(filepath.Join(projectRoot, "composer.lock"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.4"}],
  "packages-dev": []
}`), 0644); err != nil {
		t.Fatal(err)
	}
	installedDir := filepath.Join(projectRoot, "vendor", "composer")
	if err := os.MkdirAll(installedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDir, "installed.json"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.4"}]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	composerInstallCalls := 0
	defer engine.SetMagentoComposerInstallRunnerForTest(func(projectName string, config engine.Config, stdout, stderr io.Writer) error {
		composerInstallCalls++
		return nil
	})()

	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	engine.MaybeRunMagentoComposerInstallForTest("sample-project", config)

	if composerInstallCalls != 0 {
		t.Fatalf("expected composer install to be skipped, but it was called %d time(s)", composerInstallCalls)
	}
}

func TestMaybeRunMagentoComposerInstallRunsWhenVendorDoesNotSatisfyLock(t *testing.T) {
	projectRoot := t.TempDir()
	chdirForTest(t, projectRoot)
	// No composer.lock/installed.json fixtures: VendorSatisfiesComposerLock reports false.

	composerInstallCalls := 0
	defer engine.SetMagentoComposerInstallRunnerForTest(func(projectName string, config engine.Config, stdout, stderr io.Writer) error {
		composerInstallCalls++
		return nil
	})()

	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	engine.MaybeRunMagentoComposerInstallForTest("sample-project", config)

	if composerInstallCalls != 1 {
		t.Fatalf("expected composer install to run exactly once, got %d", composerInstallCalls)
	}
}

func TestMaybeRunMagentoComposerInstallRunsWhenLockVersionMismatches(t *testing.T) {
	projectRoot := t.TempDir()
	chdirForTest(t, projectRoot)

	if err := os.WriteFile(filepath.Join(projectRoot, "composer.lock"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.4"}],
  "packages-dev": []
}`), 0644); err != nil {
		t.Fatal(err)
	}
	installedDir := filepath.Join(projectRoot, "vendor", "composer")
	if err := os.MkdirAll(installedDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Installed version does not match the lock: must NOT be treated as satisfied.
	if err := os.WriteFile(filepath.Join(installedDir, "installed.json"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.3"}]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	composerInstallCalls := 0
	defer engine.SetMagentoComposerInstallRunnerForTest(func(projectName string, config engine.Config, stdout, stderr io.Writer) error {
		composerInstallCalls++
		return nil
	})()

	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	engine.MaybeRunMagentoComposerInstallForTest("sample-project", config)

	if composerInstallCalls != 1 {
		t.Fatalf("expected composer install to run exactly once, got %d", composerInstallCalls)
	}
}
