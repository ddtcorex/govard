package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
)

func TestRunStagedCreateProjectForTestPreservesGovardFiles(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".govard"), 0o755); err != nil {
		t.Fatalf("mkdir .govard: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard", "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("write .govard/keep.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write stale.txt: %v", err)
	}

	err := bootstrap.RunStagedCreateProjectForTest(projectDir, nil, func(stageDir string) error {
		if err := os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"sample\"}\n"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(stageDir, "src"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stageDir, "src", "main.js"), []byte("console.log('ok')\n"), 0o644)
	}, "")
	if err != nil {
		t.Fatalf("RunStagedCreateProjectForTest() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard", "keep.txt")); err != nil {
		t.Fatalf("expected .govard contents to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "src", "main.js")); err != nil {
		t.Fatalf("expected staged src/main.js to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.txt to be removed, got err=%v", err)
	}
}

func TestNextJSCreateProjectStagesIntoTemporaryDirectory(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write stale.txt: %v", err)
	}

	var stageDir string
	restore := bootstrap.SetNextJSStageProjectCreatorForTest(func(dir string) error {
		stageDir = dir
		return os.WriteFile(filepath.Join(dir, "package.json"), []byte("{\"name\":\"next-app\"}\n"), 0o644)
	})
	defer restore()

	nextJSBootstrap := bootstrap.NewNextJSBootstrap(bootstrap.Options{})
	if err := nextJSBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if stageDir == "" {
		t.Fatal("expected staged directory to be captured")
	}
	if stageDir == projectDir {
		t.Fatalf("expected staged directory to differ from project dir")
	}
	if filepath.Dir(stageDir) != projectDir {
		t.Fatalf("expected staged directory to live under project dir, got %s", stageDir)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.txt to be removed, got err=%v", err)
	}
}

func TestLaravelCreateProjectWithRunnerStagesComposerCreateProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var capturedCommand string
	laravelBootstrap := bootstrap.NewLaravelBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			capturedCommand = command
			stageDir := extractStageHostDir(t, command)
			return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"laravel-app\"}\n"), 0o644)
		},
	})

	if err := laravelBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if !strings.Contains(capturedCommand, `composer create-project laravel/laravel "$GOVARD_STAGE_DIR" --no-interaction`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
}

func TestNextJSCreateProjectWithRunnerStagesCreateNextApp(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var capturedCommand string
	nextJSBootstrap := bootstrap.NewNextJSBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			capturedCommand = command
			stageDir := extractStageHostDir(t, command)
			return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"nextjs-app\"}\n"), 0o644)
		},
	})

	if err := nextJSBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if !strings.Contains(capturedCommand, `npx create-next-app@latest "$GOVARD_STAGE_DIR" --typescript --tailwind --eslint --app --no-src-dir --import-alias '@/*' --use-npm --yes`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if !strings.Contains(capturedCommand, "GOVARD_STAGE_DIR='/app/") {
		t.Fatalf("expected staged dir under NodeWorkDir (/app), got: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
}

func TestWordPressCreateProjectUsesDownloaderInsteadOfWPCLI(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var downloadDir string
	restore := bootstrap.SetWordPressCoreDownloaderForTest(func(targetDir string) error {
		downloadDir = targetDir
		if err := os.WriteFile(filepath.Join(targetDir, "wp-load.php"), []byte("<?php\n"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(targetDir, "wp-config-sample.php"), []byte("<?php\n"), 0o644)
	})
	defer restore()

	wpBootstrap := bootstrap.NewWordPressBootstrap(bootstrap.Options{
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
	wpBootstrap := bootstrap.NewWordPressBootstrap(bootstrap.Options{
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

func TestShopwareInstallSyncsDomainAwareURLs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "bin", "console"), []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatalf("write console stub: %v", err)
	}

	commands := make([]string, 0, 4)
	shopwareBootstrap := bootstrap.NewShopwareBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			commands = append(commands, command)
			return nil
		},
		DBHost: "db",
		DBUser: "shopware",
		DBPass: "shopware",
		DBName: "shopware",
		Domain: "sample.test",
	})

	if err := shopwareBootstrap.Install(projectDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	envContentBytes, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envContent := string(envContentBytes)
	if !strings.Contains(envContent, "APP_URL=https://sample.test") {
		t.Fatalf("expected APP_URL to be domain-aware, got:\n%s", envContent)
	}
	if !strings.Contains(envContent, "PROXY_URL=https://sample.test") {
		t.Fatalf("expected PROXY_URL to be domain-aware, got:\n%s", envContent)
	}
	if !strings.Contains(envContent, "DATABASE_URL=mysql://shopware:shopware@db:3306/shopware") {
		t.Fatalf("expected DATABASE_URL to be rewritten, got:\n%s", envContent)
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "sales-channel:replace:url") || !strings.Contains(joined, "https://sample.test") {
		t.Fatalf("expected sales channel URL sync command, got:\n%s", joined)
	}
}

func extractStageHostDir(t *testing.T, command string) string {
	t.Helper()
	match := regexp.MustCompile(`GOVARD_STAGE_HOST_DIR='([^']+)'`).FindStringSubmatch(command)
	if len(match) != 2 {
		t.Fatalf("could not extract GOVARD_STAGE_HOST_DIR from command: %s", command)
	}
	return match[1]
}
