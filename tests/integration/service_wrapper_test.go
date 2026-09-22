//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceWrapperCommandsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("RedisUsesDefaultCacheCLI", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-redis-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "redis", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|inspect -f {{.State.Running}} m2-clone-basic-redis-1")
		assertContains(t, logs, "docker|exec m2-clone-basic-redis-1 redis-cli PING")
	})

	t.Run("RedisBareCliStaysInteractive", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-redis-cli-m2")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "redis", "cli")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec -i m2-clone-basic-redis-1 redis-cli")
	})

	t.Run("RedisSwitchesToValkeyCLI", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-redis-valkey-m2")
		overridePath := filepath.Join(projectDir, ".govard.local.yml")
		if err := os.WriteFile(overridePath, []byte("stack:\n  features:\n    redis: true\n  services:\n    cache: valkey\n"), 0o644); err != nil {
			t.Fatalf("failed to write .govard.local.yml: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "redis", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec m2-clone-basic-redis-1 valkey-cli PING")
	})

	t.Run("ValkeyGuardAndRuntime", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "service-valkey-m2")
		overridePath := filepath.Join(projectDir, ".govard.local.yml")
		if err := os.WriteFile(overridePath, []byte("stack:\n  services:\n    cache: redis\n"), 0o644); err != nil {
			t.Fatalf("failed to write .govard.local.yml: %v", err)
		}

		guardResult := env.RunGovard(t, projectDir, "env", "valkey", "PING")
		guardResult.AssertSuccess(t)
		assertContains(t, guardResult.Stdout+guardResult.Stderr, "Valkey is not enabled")

		if err := os.WriteFile(overridePath, []byte("stack:\n  features:\n    redis: true\n  services:\n    cache: valkey\n"), 0o644); err != nil {
			t.Fatalf("failed to write .govard.local.yml: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "valkey", "PING")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|exec m2-clone-basic-redis-1 valkey-cli PING")
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
		assertContains(t, logs, "docker|exec m2-clone-basic-varnish-1 varnishadm ban req.url ~ /.*")
	})
}
