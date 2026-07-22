package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"
)

func TestBootstrapPkgSymfonyFreshCommands(t *testing.T) {
	cases := []struct {
		version  string
		expected string
	}{
		{"7.0", "symfony/skeleton"},
		{"6.4", "symfony/skeleton:^6.0"},
		{"5.4", "symfony/website-skeleton:^5.0"},
		{"", "symfony/skeleton"},
	}

	for _, tc := range cases {
		opts := bootstrap.Options{Version: tc.version}
		symfony := bootstrap.NewSymfonyBootstrap(opts)
		cmds := symfony.FreshCommands()

		if len(cmds) == 0 {
			t.Fatalf("expected commands for version %s, got none", tc.version)
		}

		if !containsSubstring(cmds[0], tc.expected) {
			t.Errorf("expected command to contain %q for version %s, got %q", tc.expected, tc.version, cmds[0])
		}
	}
}

func TestBootstrapPkgSymfonyRun(t *testing.T) {
	opts := bootstrap.Options{Version: "7.0"}

	err := bootstrap.BootstrapSymfony(opts)
	if err != nil {
		t.Fatalf("BootstrapSymfony failed: %v", err)
	}
}

func TestBootstrapDispatcherSymfony(t *testing.T) {
	opts := bootstrap.DefaultOptions()
	opts.Version = "7.0"

	err := frameworks.RunBootstrap("symfony", opts)
	if err != nil {
		t.Fatalf("Run(symfony) failed: %v", err)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSymfonyDoctrineDetection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symfony-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := bootstrap.Options{Version: "7.0"}
	symfony := bootstrap.NewSymfonyBootstrap(opts)

	// Case 1: No composer.json
	if symfony.HasDoctrineForTest(tempDir) {
		t.Error("expected hasDoctrine to be false when composer.json does not exist")
	}
	if symfony.HasMigrationsForTest(tempDir) {
		t.Error("expected hasMigrations to be false when composer.json does not exist")
	}

	composerPath := filepath.Join(tempDir, "composer.json")

	// Case 2: Minimal composer.json without Doctrine
	minimalJSON := `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "7.0.*"
		}
	}`
	if err := os.WriteFile(composerPath, []byte(minimalJSON), 0644); err != nil {
		t.Fatalf("failed to write composer.json: %v", err)
	}
	if symfony.HasDoctrineForTest(tempDir) {
		t.Error("expected hasDoctrine to be false with minimal composer.json")
	}
	if symfony.HasMigrationsForTest(tempDir) {
		t.Error("expected hasMigrations to be false with minimal composer.json")
	}

	// Case 3: composer.json with doctrine/orm
	doctrineJSON := `{
		"require": {
			"php": ">=8.2",
			"symfony/orm-pack": "^2.0"
		}
	}`
	if err := os.WriteFile(composerPath, []byte(doctrineJSON), 0644); err != nil {
		t.Fatalf("failed to write composer.json: %v", err)
	}
	if !symfony.HasDoctrineForTest(tempDir) {
		t.Error("expected hasDoctrine to be true when symfony/orm-pack is present")
	}
	if symfony.HasMigrationsForTest(tempDir) {
		t.Error("expected hasMigrations to be false when only symfony/orm-pack is present")
	}

	// Case 4: composer.json with doctrine/doctrine-migrations-bundle
	migrationsJSON := `{
		"require": {
			"php": ">=8.2",
			"doctrine/doctrine-bundle": "^2.10",
			"doctrine/doctrine-migrations-bundle": "^3.2"
		}
	}`
	if err := os.WriteFile(composerPath, []byte(migrationsJSON), 0644); err != nil {
		t.Fatalf("failed to write composer.json: %v", err)
	}
	if !symfony.HasDoctrineForTest(tempDir) {
		t.Error("expected hasDoctrine to be true when doctrine/doctrine-bundle is present")
	}
	if !symfony.HasMigrationsForTest(tempDir) {
		t.Error("expected hasMigrations to be true when doctrine/doctrine-migrations-bundle is present")
	}
}
