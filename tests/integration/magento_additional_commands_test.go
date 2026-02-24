//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileCommandJSONAndApply(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "profile-m2")

	jsonResult := env.RunGovard(t, projectDir, "config", "profile", "--json")
	jsonResult.AssertSuccess(t)
	assertContains(t, jsonResult.Stdout, `"framework": "magento2"`)
	assertContains(t, jsonResult.Stdout, `"selected"`)

	applyResult := env.RunGovard(t, projectDir, "config", "profile", "apply")
	applyResult.AssertSuccess(t)

	configBytes, err := os.ReadFile(filepath.Join(projectDir, "govard.yml"))
	if err != nil {
		t.Fatalf("failed to read govard.yml: %v", err)
	}
	config := string(configBytes)
	assertContains(t, config, "project_name: m2-clone-basic")
	assertContains(t, config, "php_version:")
}

func TestSnapshotCommandsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "snapshot-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	mediaFile := filepath.Join(projectDir, "pub", "media", "catalog", "example.txt")
	if err := os.MkdirAll(filepath.Dir(mediaFile), 0o755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}
	if err := os.WriteFile(mediaFile, []byte("fixture-media"), 0o644); err != nil {
		t.Fatalf("failed to write media fixture: %v", err)
	}

	createResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "create", "test-snapshot")
	createResult.AssertSuccess(t)

	snapshotRoot := filepath.Join(projectDir, ".govard", "snapshots", "test-snapshot")
	if _, err := os.Stat(snapshotRoot); err != nil {
		t.Fatalf("expected snapshot dir to exist: %v", err)
	}

	listResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "list")
	listResult.AssertSuccess(t)
	assertContains(t, listResult.Stdout, "test-snapshot")

	restoreResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "restore", "test-snapshot", "--db-only")
	restoreResult.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|inspect -f {{range .Config.Env}}{{println .}}{{end}} m2-clone-basic-db-1")
	assertContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 mysqldump -u magento magento")
	assertContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 mysql -u magento magento")
}

func TestUpgradeCommandMessage(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "upgrade-m2")

	result := env.RunGovard(t, projectDir, "upgrade")
	result.AssertSuccess(t)
	assertContains(t, strings.ToLower(result.Stdout+result.Stderr), "not implemented yet")
}

func TestOpenCommandGuidanceTargets(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "open-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	shellResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "shell")
	shellResult.AssertSuccess(t)
	assertContains(t, shim.ReadLog(t), "docker|exec")

	sftpResult := env.RunGovard(t, projectDir, "open", "sftp")
	sftpResult.AssertSuccess(t)
	assertContains(t, sftpResult.Stdout+sftpResult.Stderr, "SFTP is not supported for local target")

	unknownResult := env.RunGovard(t, projectDir, "open", "unknown-target")
	unknownResult.AssertExitCode(t, 1)
	assertContains(t, strings.ToLower(unknownResult.Stdout+unknownResult.Stderr), "unknown target")
}

func TestDoctorJSONAndDepsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "doctor-deps-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	doctorResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "doctor", "--json")
	if doctorResult.ExitCode != 0 && doctorResult.ExitCode != 1 {
		t.Fatalf("expected doctor exit code 0 or 1, got %d\nstderr=%s", doctorResult.ExitCode, doctorResult.Stderr)
	}
	assertContains(t, doctorResult.Stdout, `"checks":`)

	depsResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "doctor", "fix-deps")
	depsResult.AssertSuccess(t)
	assertContains(t, depsResult.Stdout+depsResult.Stderr, "All required dependencies are available.")
}

func TestDebugStatusAndShellDisabled(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "debug-m2")

	statusResult := env.RunGovard(t, projectDir, "debug", "status")
	statusResult.AssertSuccess(t)
	assertContains(t, strings.ToLower(statusResult.Stdout+statusResult.Stderr), "xdebug is currently")

	shellResult := env.RunGovard(t, projectDir, "debug", "shell")
	shellResult.AssertSuccess(t)
	assertContains(t, shellResult.Stdout+shellResult.Stderr, "Xdebug is disabled")
}

func TestStopCommandRunsHooksWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "stop-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	localOverride := `hooks:
  pre_stop:
    - name: pre stop marker
      run: "echo pre >> .govard-stop-hooks.log"
  post_stop:
    - name: post stop marker
      run: "echo post >> .govard-stop-hooks.log"
`
	overridePath := filepath.Join(projectDir, "govard.local.yml")
	if err := os.WriteFile(overridePath, []byte(localOverride), 0o644); err != nil {
		t.Fatalf("failed to write govard.local.yml: %v", err)
	}

	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "stop")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|compose --project-directory")
	assertContains(t, logs, " stop")

	content, err := os.ReadFile(filepath.Join(projectDir, ".govard-stop-hooks.log"))
	if err != nil {
		t.Fatalf("failed to read stop hook log: %v", err)
	}
	if strings.TrimSpace(string(content)) != "pre\npost" {
		t.Fatalf("expected stop hook order pre->post, got:\n%s", string(content))
	}
}

func TestUpQuickstartWithShims(t *testing.T) {
	SkipIfNoDocker(t)

	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "up-m2")
	CopyBlueprints(t, env.BlueprintsPath, filepath.Join(projectDir, "blueprints"))

	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "up", "--quickstart")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|compose --project-directory")
	assertContains(t, logs, " up -d")
}

func TestServiceWrapperCommandsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("RedisUsesDefaultCacheCLI", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-redis-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "redis", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|inspect -f {{.State.Running}} m2-clone-basic-redis-1")
		assertContains(t, logs, "docker|exec -it m2-clone-basic-redis-1 valkey-cli PING")
	})

	t.Run("RedisSwitchesToValkeyCLI", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-redis-valkey-m2")
		overridePath := filepath.Join(projectDir, "govard.local.yml")
		if err := os.WriteFile(overridePath, []byte("stack:\n  services:\n    cache: valkey\n"), 0o644); err != nil {
			t.Fatalf("failed to write govard.local.yml: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "redis", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -it m2-clone-basic-redis-1 valkey-cli PING")
	})

	t.Run("ValkeyGuardAndRuntime", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-valkey-m2")
		overridePath := filepath.Join(projectDir, "govard.local.yml")
		if err := os.WriteFile(overridePath, []byte("stack:\n  services:\n    cache: redis\n"), 0o644); err != nil {
			t.Fatalf("failed to write govard.local.yml: %v", err)
		}

		guardResult := env.RunGovard(t, projectDir, "env", "valkey", "PING")
		guardResult.AssertSuccess(t)
		assertContains(t, guardResult.Stdout+guardResult.Stderr, "Valkey is not enabled")

		if err := os.WriteFile(overridePath, []byte("stack:\n  services:\n    cache: valkey\n"), 0o644); err != nil {
			t.Fatalf("failed to write govard.local.yml: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "valkey", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -it m2-clone-basic-redis-1 valkey-cli PING")
	})

	t.Run("SearchServiceCommandsUseCurl", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-search-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		esResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "elasticsearch", "_cluster/health")
		esResult.AssertSuccess(t)

		osResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "opensearch", "_cat/indices")
		osResult.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -i m2-clone-basic-elasticsearch-1 curl -s -X GET http://localhost:9200/_cluster/health")
		assertContains(t, logs, "docker|exec -i m2-clone-basic-elasticsearch-1 curl -s -X GET http://localhost:9200/_cat/indices")
	})

	t.Run("VarnishBanBuildsExpectedCommand", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-varnish-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "varnish", "ban", "/.*")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -it m2-clone-basic-varnish-1 varnishadm ban req.url ~ /.*")
	})
}

func TestOpenAndShortcutBrowserCommands(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "open-shortcuts-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	installRuntimeCommandShim(t, shim, "xdg-open", 0)

	commands := [][]string{
		{"open", "admin"},
		{"open", "db"},
		{"open", "elasticsearch"},
		{"open", "opensearch"},
		{"open", "mail"},
		{"open", "pma"},
	}

	for _, args := range commands {
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), args...)
		result.AssertSuccess(t)
	}

	ok := WaitForCondition(t, 2*time.Second, 20*time.Millisecond, func() bool {
		logs := shim.ReadLog(t)
		return strings.Count(logs, "xdg-open|") >= len(commands)
	})
	if !ok {
		t.Fatalf("expected %d xdg-open invocations, got logs:\n%s", len(commands), shim.ReadLog(t))
	}

	logs := shim.ReadLog(t)
	assertContains(t, logs, "xdg-open|https://m2-clone-basic.test/admin")
	assertContains(t, logs, "xdg-open|https://pma.govard.test")
	assertContains(t, logs, "xdg-open|https://elasticsearch.govard.test")
	assertContains(t, logs, "xdg-open|https://opensearch.govard.test")
	assertContains(t, logs, "xdg-open|https://mail.govard.test")
	if strings.Contains(logs, "ssh|") {
		t.Fatalf("did not expect ssh tunnel for default open targets, got logs:\n%s", logs)
	}
}

func TestOpenDBEnvironmentSelection(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "open-db-env-selection-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	installRuntimeCommandShim(t, shim, "xdg-open", 0)

	localResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "db", "-e", "local")
	localResult.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "xdg-open|https://pma.govard.test")
	if strings.Contains(logs, "ssh|") {
		t.Fatalf("did not expect ssh tunnel for local db open, got logs:\n%s", logs)
	}

	unknownResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "db", "-e", "missing")
	unknownResult.AssertExitCode(t, 1)
	assertContains(t, strings.ToLower(unknownResult.Stdout+unknownResult.Stderr), "unknown remote environment")

	defaultResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "db")
	defaultResult.AssertSuccess(t)
	updatedLogs := shim.ReadLog(t)
	assertContains(t, updatedLogs, "xdg-open|https://pma.govard.test")
	if strings.Contains(updatedLogs, "ssh|") {
		t.Fatalf("did not expect ssh tunnel when open db uses default local env, got logs:\n%s", updatedLogs)
	}
}

func TestOpenTargetsRemoteEnvironment(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "open-remote-targets-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	installRuntimeCommandShim(t, shim, "xdg-open", 0)

	adminResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "admin", "-e", "dev")
	adminResult.AssertSuccess(t)

	sftpResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "sftp", "-e", "dev")
	sftpResult.AssertSuccess(t)

	shellResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "shell", "-e", "dev")
	shellResult.AssertSuccess(t)

	searchResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "open", "elasticsearch", "-e", "dev")
	searchResult.AssertExitCode(t, 1)
	assertContains(t, strings.ToLower(searchResult.Stdout+searchResult.Stderr), "not supported yet")

	logs := shim.ReadLog(t)
	assertContains(t, logs, "xdg-open|https://dev.example.com/")
	assertContains(t, logs, "xdg-open|sftp://deploy@dev.example.com:22/var/www/html")
	assertContains(t, logs, "ssh|")
}

func installRuntimeCommandShim(t *testing.T, shims *RuntimeShims, name string, exitCode int) {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
set -eu
log="${GOVARD_TEST_RUNTIME_LOG:-}"
if [ -n "$log" ]; then
  printf '%%s|%%s\n' %q "$*" >> "$log"
fi
exit %d
`, name, exitCode)

	path := filepath.Join(shims.Dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write %s shim: %v", name, err)
	}
}
