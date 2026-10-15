package tests

import (
	"fmt"
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
	if !sameTestFile(t, entries[0].Path, root) {
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

func TestDesktopPkgOnboardProjectFromGitForPathForTestClonesBeforeInit(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	steps := make([]string, 0, 3)
	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		steps = append(steps, "validate")
		if protocol != "https" {
			t.Fatalf("expected https protocol, got %s", protocol)
		}
		if repoURL != "https://example.com/acme/shop.git" {
			t.Fatalf("unexpected repo URL: %s", repoURL)
		}
		return nil
	})
	defer restoreValidate()

	restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
		steps = append(steps, "clone")
		if destination != root {
			t.Fatalf("expected destination %s, got %s", root, destination)
		}
		return nil
	})
	defer restoreClone()

	restoreInit := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		steps = append(steps, "init")
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
	defer restoreInit()

	message, err := desktop.OnboardProjectFromGitForPathForTest(
		root,
		"laravel",
		"https",
		"https://example.com/acme/shop.git",
	)
	if err != nil {
		t.Fatalf("onboard git project: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "initialized") {
		t.Fatalf("expected initialized message, got %q", message)
	}
	expectedSteps := []string{"validate", "clone", "init"}
	if !reflect.DeepEqual(steps, expectedSteps) {
		t.Fatalf("unexpected step order: %#v", steps)
	}
}

func TestDesktopPkgOnboardProjectFromGitForPathForTestShowsSSHSetupGuidanceOnValidationFail(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		return fmt.Errorf("permission denied (publickey)")
	})
	defer restoreValidate()

	restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
		t.Fatalf("clone should not run when validation fails")
		return nil
	})
	defer restoreClone()

	restoreInit := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		t.Fatalf("init should not run when git validation fails")
		return "", nil
	})
	defer restoreInit()

	_, err := desktop.OnboardProjectFromGitForPathForTest(
		root,
		"laravel",
		"ssh",
		"git@example.com:acme/shop.git",
	)
	if err == nil {
		t.Fatal("expected git validation error")
	}
	lowered := strings.ToLower(err.Error())
	if !strings.Contains(lowered, "git ssh connection validation failed") {
		t.Fatalf("expected ssh validation error message, got %v", err)
	}
	if !strings.Contains(lowered, "ssh-add -l") {
		t.Fatalf("expected ssh setup guidance, got %v", err)
	}
	if !strings.Contains(lowered, "ssh -t git@example.com") && !strings.Contains(lowered, "ssh -t git@") {
		t.Fatalf("expected ssh verify guidance, got %v", err)
	}
}

func TestDesktopPkgOnboardProjectFromGitForPathForTestRequiresFolderOverrideConfirmation(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		t.Fatalf("git validation should not run without override confirmation")
		return nil
	})
	defer restoreValidate()

	_, err := desktop.OnboardProjectFromGitWithConfirmationForPathForTest(
		root,
		"laravel",
		"https",
		"https://example.com/acme/shop.git",
		false,
	)
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "confirm folder override") {
		t.Fatalf("expected confirmation guidance, got %v", err)
	}
}

func TestDesktopPkgOnboardProjectFromGitForPathForTestClearsFolderBeforeClone(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	legacyFile := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(legacyFile, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	legacyDir := filepath.Join(root, "old")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		return nil
	})
	defer restoreValidate()

	restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
		if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
			t.Fatalf("expected legacy file to be removed before clone")
		}
		if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
			t.Fatalf("expected legacy directory to be removed before clone")
		}
		return nil
	})
	defer restoreClone()

	restoreInit := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
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
	defer restoreInit()

	_, err := desktop.OnboardProjectFromGitForPathForTest(
		root,
		"laravel",
		"https",
		"https://example.com/acme/shop.git",
	)
	if err != nil {
		t.Fatalf("onboard git project: %v", err)
	}
}

func TestDesktopPkgOnboardProjectFromGitForPathForTestDoesNotClearFolderWhenValidationFails(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	legacyFile := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(legacyFile, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		return fmt.Errorf("auth failed")
	})
	defer restoreValidate()

	restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
		t.Fatalf("clone should not run when validation fails")
		return nil
	})
	defer restoreClone()

	_, err := desktop.OnboardProjectFromGitForPathForTest(
		root,
		"laravel",
		"https",
		"https://example.com/acme/shop.git",
	)
	if err == nil {
		t.Fatal("expected git validation error")
	}
	if _, statErr := os.Stat(legacyFile); statErr != nil {
		t.Fatalf("expected legacy file to remain after validation failure, got %v", statErr)
	}
}

func TestDesktopPkgOnboardProjectFromGitForPathForTestRejectsDangerousDestinationPath(t *testing.T) {
	home := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	t.Setenv("HOME", home)

	restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
		return nil
	})
	defer restoreValidate()

	restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
		t.Fatalf("clone should not run for dangerous destination path")
		return nil
	})
	defer restoreClone()

	_, err := desktop.OnboardProjectFromGitForPathForTest(
		home,
		"laravel",
		"https",
		"https://example.com/acme/shop.git",
	)
	if err == nil {
		t.Fatal("expected dangerous path validation error")
	}
	lowered := strings.ToLower(err.Error())
	if !strings.Contains(lowered, "refusing to clone into home directory") {
		t.Fatalf("expected home directory safety error, got %v", err)
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
  db: mariadb
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
    redis: true
    elasticsearch: true
    rabbitmq: true
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

func TestDesktopPkgOnboardProjectForPathForTestPassesFrameworkVersion(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	var initArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		if dir != root {
			t.Fatalf("unexpected command dir %s", dir)
		}
		initArgs = append([]string{}, args...)
		content := strings.TrimSpace(`
project_name: framework-version-demo
framework: laravel
domain: framework-version-demo.test
`) + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(content), 0o644); err != nil {
			return "", err
		}
		return "init ok", nil
	})
	defer restore()

	message, err := desktop.OnboardProjectWithOptionsForPathForTest(
		root,
		"laravel",
		"",
		false,
		true,
		false,
		true,
		"11",
	)
	if err != nil {
		t.Fatalf("onboard project: %v", err)
	}

	expected := []string{"init", "--framework", "laravel", "--framework-version", "11"}
	if !reflect.DeepEqual(initArgs, expected) {
		t.Fatalf("unexpected init args: %#v", initArgs)
	}
	if !strings.Contains(strings.ToLower(message), "initialized") {
		t.Fatalf("expected initialized message, got %q", message)
	}
}

func TestDesktopPkgOnboardProjectForPathForTestDoesNotAutoBootstrapWhenRemotesExist(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

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

	if len(calls) != 0 {
		t.Fatalf("expected no post-onboarding commands, got %d", len(calls))
	}
	if strings.Contains(strings.ToLower(message), "auto bootstrap") {
		t.Fatalf("did not expect auto bootstrap summary in message, got %q", message)
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

func TestDesktopPkgOnboardProjectForPathForTestAutoMigrateFromDDEV(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	ddevDir := filepath.Join(root, ".ddev")
	if err := os.MkdirAll(ddevDir, 0o755); err != nil {
		t.Fatalf("create .ddev dir: %v", err)
	}
	ddevConfig := strings.TrimSpace(`
name: ddev-demo
type: magento2
php_version: "8.3"
`) + "\n"
	if err := os.WriteFile(filepath.Join(ddevDir, "config.yaml"), []byte(ddevConfig), 0o644); err != nil {
		t.Fatalf("write .ddev/config.yaml: %v", err)
	}

	var initArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		if dir != root {
			t.Fatalf("unexpected command dir %s", dir)
		}
		initArgs = append([]string{}, args...)
		content := strings.TrimSpace(`
project_name: ddev-demo
framework: magento2
domain: ddev-demo.test
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
	expected := []string{"init", "--framework", "magento2", "--migrate-from", "ddev"}
	if !reflect.DeepEqual(initArgs, expected) {
		t.Fatalf("unexpected init args: %#v", initArgs)
	}
	if !strings.Contains(strings.ToLower(message), "initialized") {
		t.Fatalf("expected initialized message, got %q", message)
	}
}
