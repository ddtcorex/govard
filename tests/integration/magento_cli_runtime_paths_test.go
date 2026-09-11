//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestInitPreservesRemotesAndHooks(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "init-merge-m2")

	configPath := filepath.Join(projectDir, ".govard.yml")
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read .govard.yml: %v", err)
	}

	original = append(original, []byte(`
hooks:
  pre_up:
    - name: fixture-pre-up
      run: "echo before-up"
`)...)
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatalf("failed to update .govard.yml: %v", err)
	}

	result := env.RunGovard(t, projectDir, "init", "--framework", "magento2", "--framework-version", "2.4.7-p3")
	result.AssertSuccess(t)

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("failed to load generated config: %v", err)
	}

	if config.Framework != "magento2" {
		t.Fatalf("expected framework=magento2, got %q", config.Framework)
	}
	if config.FrameworkVersion != "2.4.7-p3" {
		t.Fatalf("expected framework_version=2.4.7-p3, got %q", config.FrameworkVersion)
	}
	if config.Remotes["dev"].Host != "dev.example.com" {
		t.Fatalf("expected remotes.dev to be preserved, got: %#v", config.Remotes["dev"])
	}
	if config.Remotes["production"].Protected == nil || !*config.Remotes["production"].Protected {
		t.Fatalf("expected remotes.production.protected=true, got: %#v", config.Remotes["production"])
	}
	if len(config.Hooks["pre_up"]) != 1 || config.Hooks["pre_up"][0].Name != "fixture-pre-up" {
		t.Fatalf("expected hooks.pre_up to be preserved, got: %#v", config.Hooks["pre_up"])
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("expected compose file at %s: %v", composePath, err)
	}
}

func TestStatusFailsFastWhenDockerDaemonUnreachable(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "status-m2")

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		[]string{"DOCKER_HOST=unix:///tmp/govard-int-status-missing.sock"},
		"status",
	)

	// An unreachable daemon is a missing capability: the command must fail
	// before its workflow starts rather than mid-way through listing containers.
	if result.ExitCode != 3 {
		t.Fatalf("expected exit code 3 (missing capability), got %d\nstdout=%s\nstderr=%s",
			result.ExitCode, result.Stdout, result.Stderr)
	}
	output := result.Stdout + result.Stderr
	assertContains(t, output, `missing capability "docker"`)
}

func TestShellFallsBackToShWhenBashFails(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "shell-fallback-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 127, "ssh": 0, "rsync": 0})

	// The one-shot failure must target the in-container bash attempt, not the
	// capability probe's `docker compose version` call.
	shellEnv := append(shim.Env(), "GOVARD_TEST_FAIL_MATCH_DOCKER=bash")

	result := env.RunGovardWithEnv(t, projectDir, shellEnv, "shell")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|exec")
	assertContains(t, logs, "bash -c export PS1=")
	assertContains(t, logs, "sh")
}

func TestLogsCommandPaths(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("DefaultLogsUsesDockerCompose", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "logs-default-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "logs")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|compose --project-directory")
		assertContains(t, logs, " logs")
	})

	t.Run("ErrorFilterUsesShellPipeline", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "logs-errors-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installShellShim(t, shim)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "logs", "--errors")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "sh|-c docker compose --project-directory")
		assertContains(t, logs, "grep -iE 'error|critical|fail|exception'")
	})
}

func TestFrameworkCommandRuntimeForMagentoProject(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("MagentoCommandUsesPhpContainer", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "framework-magento-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "tool", "magento", "cache:flush")
		result.AssertSuccess(t)

		config, _, err := engine.LoadConfigFromDir(projectDir, true)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		expectedUser := "www-data"
		if config.Stack.UserID > 0 && config.Stack.GroupID > 0 {
			expectedUser = fmt.Sprintf("%d:%d", config.Stack.UserID, config.Stack.GroupID)
		}

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -i -u "+expectedUser+" -w /var/www/html m2-clone-basic-php-1 php bin/magento cache:flush")
	})

	t.Run("ComposerUsesConfiguredUserAndGroup", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "framework-composer-user-m2")

		localOverride := `stack:
  user_id: 2000
  group_id: 2001
`
		overridePath := filepath.Join(projectDir, ".govard.local.yml")
		if err := os.WriteFile(overridePath, []byte(localOverride), 0o644); err != nil {
			t.Fatalf("failed to write .govard.local.yml: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "tool", "composer", "install", "--no-dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -i -u 2000:2001 -w /var/www/html m2-clone-basic-php-1 composer install --no-dev")
	})
}

func installShellShim(t *testing.T, shims *RuntimeShims) {
	t.Helper()
	script := `#!/bin/sh
set -eu
log="${GOVARD_TEST_RUNTIME_LOG:-}"
if [ -n "$log" ]; then
  printf '%s|%s\n' "sh" "$*" >> "$log"
fi
exit "${GOVARD_TEST_EXIT_SH:-0}"
`
	path := filepath.Join(shims.Dir, "sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to install sh shim: %v", err)
	}
}
