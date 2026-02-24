package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestMigrateFromDDEV(t *testing.T) {
	tempDir := t.TempDir()
	ddevDir := filepath.Join(tempDir, ".ddev")
	if err := os.MkdirAll(ddevDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYaml := `name: test-ddev
type: magento2
php_version: "8.2"
database:
  type: mariadb
  version: "10.6"
`
	if err := os.WriteFile(filepath.Join(ddevDir, "config.yaml"), []byte(configYaml), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.MigrateFromDDEV(tempDir)
	if err != nil {
		t.Fatalf("MigrateFromDDEV failed: %v", err)
	}

	if result.ProjectName != "test-ddev" {
		t.Errorf("expected ProjectName=test-ddev, got %q", result.ProjectName)
	}
	if result.Recipe != "magento2" {
		t.Errorf("expected Recipe=magento2, got %q", result.Recipe)
	}
	if result.PHPVersion != "8.2" {
		t.Errorf("expected PHPVersion=8.2, got %q", result.PHPVersion)
	}
	if result.DBType != "mariadb" {
		t.Errorf("expected DBType=mariadb, got %q", result.DBType)
	}
	if result.DBVersion != "10.6" {
		t.Errorf("expected DBVersion=10.6, got %q", result.DBVersion)
	}
}

func TestMigrateFromWarden(t *testing.T) {
	tempDir := t.TempDir()

	dotEnv := `WARDEN_ENV_NAME=test-warden
WARDEN_ENV_TYPE=laravel
WARDEN_WEB_ROOT=public
WARDEN_SSH_HOST=ssh.example.com
WARDEN_SSH_USER=admin
WARDEN_SSH_PATH=/var/www/laravel
`
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(dotEnv), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.MigrateFromWarden(tempDir)
	if err != nil {
		t.Fatalf("MigrateFromWarden failed: %v", err)
	}

	if result.ProjectName != "test-warden" {
		t.Errorf("expected ProjectName=test-warden, got %q", result.ProjectName)
	}
	if result.Recipe != "laravel" {
		t.Errorf("expected Recipe=laravel, got %q", result.Recipe)
	}
	if result.WebRoot != "public" {
		t.Errorf("expected WebRoot=public, got %q", result.WebRoot)
	}

	remote, ok := result.Remotes["production"]
	if !ok {
		t.Fatal("expected production remote to be migrated")
	}
	if remote.Host != "ssh.example.com" {
		t.Errorf("expected remote Host=ssh.example.com, got %q", remote.Host)
	}
	if remote.User != "admin" {
		t.Errorf("expected remote User=admin, got %q", remote.User)
	}
	if remote.Path != "/var/www/laravel" {
		t.Errorf("expected remote Path=/var/www/laravel, got %q", remote.Path)
	}
}
