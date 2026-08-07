package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/drupal"
)

func TestBootstrapPkgDrupalFreshCommands(t *testing.T) {
	cases := []struct {
		version  string
		expected string
	}{
		{"11", "drupal/recommended-project"},
		{"10", "drupal/recommended-project:^10"},
		{"9", "drupal/recommended-project:^9"},
		{"", "drupal/recommended-project"},
	}

	for _, tc := range cases {
		opts := bootstrap.Options{Version: tc.version}
		drupal := drupal.NewDrupalBootstrap(opts)
		cmds := drupal.FreshCommands()

		if len(cmds) == 0 {
			t.Fatalf("expected commands for version %s, got none", tc.version)
		}

		if !containsSubstring(cmds[0], tc.expected) {
			t.Errorf("expected command to contain %q for version %s, got %q", tc.expected, tc.version, cmds[0])
		}
	}
}

func TestDrupalInstallUsesSharedAdminCredentials(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "web", "sites", "default"), 0o755); err != nil {
		t.Fatalf("mkdir sites/default: %v", err)
	}

	// runDrushCommand skips invoking Runner entirely unless a drush binary
	// is present on disk (it os.Stat()s vendor/bin/drush /
	// web/vendor/bin/drush before calling bootstrap.RunPHPProjectScript),
	// so the site:install command (which carries the admin credentials)
	// never reaches the recorded commands without this stub.
	drushDir := filepath.Join(projectDir, "vendor", "bin")
	if err := os.MkdirAll(drushDir, 0o755); err != nil {
		t.Fatalf("mkdir vendor/bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(drushDir, "drush"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write drush stub: %v", err)
	}

	var commands []string
	drupalBootstrap := drupal.NewDrupalBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			commands = append(commands, command)
			return nil
		},
	})

	if err := drupalBootstrap.Install(projectDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "--account-name="+conventions.DefaultAdminUser) {
		t.Errorf("expected shared default admin user in install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "--account-pass="+conventions.DefaultAdminPassword) {
		t.Errorf("expected shared default admin password in install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "--account-mail="+conventions.DefaultAdminEmail) {
		t.Errorf("expected shared default admin email in install command, got:\n%s", joined)
	}
	if strings.Contains(joined, "--account-pass=admin'") {
		t.Errorf("expected the old hardcoded 'admin' password to be gone, got:\n%s", joined)
	}
}
