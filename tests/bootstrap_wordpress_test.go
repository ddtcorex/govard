package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/wordpress"
)

func TestWordPressCreateProjectUsesDownloaderInsteadOfWPCLI(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var downloadDir string
	restore := wordpress.SetWordPressCoreDownloaderForTest(func(targetDir string) error {
		downloadDir = targetDir
		if err := os.WriteFile(filepath.Join(targetDir, "wp-load.php"), []byte("<?php\n"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(targetDir, "wp-config-sample.php"), []byte("<?php\n"), 0o644)
	})
	defer restore()

	wpBootstrap := wordpress.NewWordPressBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			return fmt.Errorf("runner should not be called during WordPress create: %s", command)
		},
	})

	if err := wpBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	expectedAppDir := projectDir
	if downloadDir != expectedAppDir {
		t.Fatalf("expected downloader target %s, got %s", expectedAppDir, downloadDir)
	}
	if _, err := os.Stat(filepath.Join(expectedAppDir, "wp-load.php")); err != nil {
		t.Fatalf("expected wp-load.php to exist after download stub: %v", err)
	}
}

func TestWordPressInstallUsesPHPScriptInsteadOfWPCLI(t *testing.T) {
	projectDir := t.TempDir()
	appDir := projectDir
	wpConfigSample := `<?php
define( 'DB_NAME', 'database_name_here' );
define( 'DB_USER', 'username_here' );
define( 'DB_PASSWORD', 'password_here' );
define( 'DB_HOST', 'localhost' );
define( 'AUTH_KEY',         'put your unique phrase here' );
require_once ABSPATH . 'wp-settings.php';
`
	if err := os.WriteFile(filepath.Join(appDir, "wp-config-sample.php"), []byte(wpConfigSample), 0o644); err != nil {
		t.Fatalf("write wp-config-sample.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "wp-load.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write wp-load.php: %v", err)
	}

	commands := make([]string, 0, 4)
	wpBootstrap := wordpress.NewWordPressBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			commands = append(commands, command)
			return nil
		},
		DBHost: "db",
		DBUser: "wordpress",
		DBPass: "wordpress",
		DBName: "wordpress",
		Domain: "sample.test",
	})

	if err := wpBootstrap.Install(projectDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(appDir, "wp-config.php")); err != nil {
		t.Fatalf("expected wp-config.php to be created: %v", err)
	}

	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "wp core") || strings.Contains(joined, "wp config create") {
		t.Fatalf("expected PHP one-liners instead of wp-cli commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "php -r") {
		t.Fatalf("expected php -r commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "/var/www/html/wp-load.php") || !strings.Contains(joined, "wp_install(") {
		t.Fatalf("expected wp-load.php / wp_install() in runner commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, strconv.Quote(conventions.DefaultAdminUser)) {
		t.Fatalf("expected default admin user in runner commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, strconv.Quote(conventions.DefaultAdminEmail)) {
		t.Fatalf("expected default admin email in runner commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, strconv.Quote(conventions.DefaultAdminPassword)) {
		t.Fatalf("expected default admin password in runner commands, got:\n%s", joined)
	}
}
