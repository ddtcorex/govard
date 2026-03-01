package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func TestDesktopPkgOnboardProjectForPathForTestInitializesWhenMissingConfig(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	var called bool
	var gotDir string
	var gotArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		called = true
		gotDir = dir
		gotArgs = append([]string{}, args...)
		content := strings.TrimSpace(`
project_name: demo
framework: laravel
domain: demo.test
`) + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(content), 0o644); err != nil {
			return "", err
		}
		return "ok", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectForPathForTest(root, "laravel")
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}
	if !called {
		t.Fatal("expected init command to run for missing .govard.yml")
	}
	if gotDir != root {
		t.Fatalf("expected init dir %s, got %s", root, gotDir)
	}
	expectedArgs := []string{"init", "--framework", "laravel"}
	if !reflect.DeepEqual(gotArgs, expectedArgs) {
		t.Fatalf("unexpected init args: %#v", gotArgs)
	}
	if !strings.Contains(strings.ToLower(message), "initialized") {
		t.Fatalf("expected initialized message, got %q", message)
	}

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one registry entry, got %d", len(entries))
	}
	if entries[0].Path != root {
		t.Fatalf("expected registry path %s, got %s", root, entries[0].Path)
	}
	if entries[0].ProjectName != "demo" {
		t.Fatalf("expected project name demo, got %s", entries[0].ProjectName)
	}
	if entries[0].Framework != "laravel" {
		t.Fatalf("expected framework laravel, got %s", entries[0].Framework)
	}
}

func TestDesktopPkgOnboardProjectForPathForTestAddsConfiguredProjectWithoutInit(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	content := strings.TrimSpace(`
project_name: shop
framework: magento2
domain: shop.test
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		t.Fatalf("did not expect init command for preconfigured project; dir=%s args=%#v", dir, args)
		return "", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectForPathForTest(root, "")
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "added") {
		t.Fatalf("expected added message, got %q", message)
	}

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one registry entry, got %d", len(entries))
	}
	if entries[0].ProjectName != "shop" {
		t.Fatalf("expected project name shop, got %s", entries[0].ProjectName)
	}
	if entries[0].Framework != "magento2" {
		t.Fatalf("expected framework magento2, got %s", entries[0].Framework)
	}
}

func TestDesktopPkgOnboardProjectWithOptionsForPathForTestAppliesOverrides(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	content := strings.TrimSpace(`
project_name: shop
framework: laravel
domain: shop.test
stack:
  php_version: "8.3"
  node_version: "22"
  db_type: mariadb
  db_version: "10.6"
  web_root: /public
  services:
    web_server: nginx
    search: none
    cache: none
    queue: none
  features:
    xdebug: true
    varnish: false
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		t.Fatalf("did not expect init command for preconfigured project; dir=%s args=%#v", dir, args)
		return "", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectWithOptionsForPathForTest(
		root,
		"",
		"custom-shop",
		true,
		true,
		true,
		true,
	)
	if err != nil {
		t.Fatalf("onboard project with options: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "added") {
		t.Fatalf("expected added message, got %q", message)
	}

	cfg, err := engine.LoadBaseConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	expectedDomain := "custom-shop.test"
	// Verify custom domain was applied
	if cfg.Domain != "custom-shop.test" {
		t.Errorf("expected domain custom-shop.test, got %s", cfg.Domain)
	}

	if !cfg.Stack.Features.Varnish {
		t.Fatalf("expected varnish enabled after onboarding override")
	}
	if cfg.Stack.Services.Cache != "redis" {
		t.Fatalf("expected cache redis, got %s", cfg.Stack.Services.Cache)
	}
	if cfg.Stack.Services.Queue != "rabbitmq" {
		t.Fatalf("expected queue rabbitmq, got %s", cfg.Stack.Services.Queue)
	}
	if cfg.Stack.Services.Search != "elasticsearch" {
		t.Fatalf("expected search elasticsearch, got %s", cfg.Stack.Services.Search)
	}

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one registry entry, got %d", len(entries))
	}
	if entries[0].Domain != expectedDomain {
		t.Fatalf("expected registry domain %s, got %s", expectedDomain, entries[0].Domain)
	}

}

func TestDesktopPkgOnboardProjectWithOptionsForPathForTestRejectsDuplicateDomain(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	content := strings.TrimSpace(`
project_name: shop
framework: magento2
domain: shop.test
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        filepath.Join(t.TempDir(), "existing"),
		ProjectName: "existing",
		Domain:      "existing.test",
		Framework:   "laravel",
		LastSeenAt:  time.Now().UTC(),
		LastCommand: "desktop-onboard",
	}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	_, err := desktop.OnboardProjectWithOptionsForPathForTest(
		root,
		"magento2",
		"existing.test",
		false,
		true,
		false,
		true,
	)
	if err == nil {
		t.Fatal("expected duplicate domain onboarding error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already used") {
		t.Fatalf("expected duplicate domain error, got %v", err)
	}
}

func TestDesktopPkgOnboardProjectForPathForTestAutoMigrateFromWarden(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	envContent := strings.TrimSpace(`
WARDEN_ENV_NAME=warden-demo
WARDEN_ENV_TYPE=magento2
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	var initArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		if dir != root {
			t.Fatalf("unexpected command dir %s", dir)
		}
		initArgs = append([]string{}, args...)
		content := strings.TrimSpace(`
project_name: warden-demo
framework: magento2
domain: warden-demo.test
`) + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(content), 0o644); err != nil {
			return "", err
		}
		return "init ok", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectForPathForTest(root, "magento2")
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}
	expected := []string{"init", "--framework", "magento2", "--migrate-from", "warden"}
	if !reflect.DeepEqual(initArgs, expected) {
		t.Fatalf("unexpected init args: %#v", initArgs)
	}
	if !strings.Contains(strings.ToLower(message), "initialized") {
		t.Fatalf("expected initialized message, got %q", message)
	}
}

func TestDesktopPkgOnboardProjectForPathForTestAutoBootstrapFromStagingRemote(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	t.Setenv("GOVARD_DESKTOP_BOOTSTRAP_SYNC", "1")

	content := strings.TrimSpace(`
project_name: shop
framework: magento2
domain: shop.test
remotes:
  staging:
    host: staging.example.com
    user: deploy
    port: 22
    path: /var/www/staging
    capabilities:
      files: true
      media: true
      db: true
      deploy: false
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var calls [][]string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		if dir != root {
			t.Fatalf("unexpected command dir %s", dir)
		}
		calls = append(calls, append([]string{}, args...))
		return "bootstrap completed", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectForPathForTest(root, "")
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly one post-onboarding command, got %d", len(calls))
	}
	expectedBootstrap := []string{"bootstrap", "--environment", "staging"}
	if !reflect.DeepEqual(calls[0], expectedBootstrap) {
		t.Fatalf("unexpected bootstrap args: %#v", calls[0])
	}
	if !strings.Contains(strings.ToLower(message), "auto bootstrap") {
		t.Fatalf("expected auto bootstrap summary in message, got %q", message)
	}
}

func TestDesktopPkgOnboardProjectForPathForTestSkipsAutoBootstrapWithoutStagingRemote(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	content := strings.TrimSpace(`
project_name: shop
framework: magento2
domain: shop.test
remotes:
  production:
    host: prod.example.com
    user: deploy
    port: 22
    path: /var/www/prod
    capabilities:
      files: true
      media: true
      db: true
      deploy: false
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		t.Fatalf("did not expect bootstrap command when staging remote is missing; dir=%s args=%#v", dir, args)
		return "", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectForPathForTest(root, "")
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}
	if strings.Contains(strings.ToLower(message), "auto bootstrap") {
		t.Fatalf("did not expect auto bootstrap summary in message, got %q", message)
	}
}
