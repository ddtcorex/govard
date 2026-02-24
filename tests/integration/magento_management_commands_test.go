//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestConfigCommandGetSetAndUnknownKey(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "config-cmd-m2")

	getProject := env.RunGovard(t, projectDir, "config", "get", "project_name")
	getProject.AssertSuccess(t)
	if strings.TrimSpace(getProject.Stdout) != "m2-clone-basic" {
		t.Fatalf("expected project_name m2-clone-basic, got %q", strings.TrimSpace(getProject.Stdout))
	}

	setDomain := env.RunGovard(t, projectDir, "config", "set", "domain", "m2-config-updated.test")
	setDomain.AssertSuccess(t)
	assertContains(t, setDomain.Stdout+setDomain.Stderr, "Config updated: domain = m2-config-updated.test")

	getDomain := env.RunGovard(t, projectDir, "config", "get", "domain")
	getDomain.AssertSuccess(t)
	if strings.TrimSpace(getDomain.Stdout) != "m2-config-updated.test" {
		t.Fatalf("expected updated domain in config get, got %q", strings.TrimSpace(getDomain.Stdout))
	}

	setCache := env.RunGovard(t, projectDir, "config", "set", "services.cache", "redis")
	setCache.AssertSuccess(t)
	assertContains(t, setCache.Stdout+setCache.Stderr, "Config updated: services.cache = redis")

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("failed to load updated config: %v", err)
	}
	if config.Domain != "m2-config-updated.test" {
		t.Fatalf("expected config domain m2-config-updated.test, got %q", config.Domain)
	}
	if config.Stack.Services.Cache != "redis" {
		t.Fatalf("expected cache service redis, got %q", config.Stack.Services.Cache)
	}

	unknown := env.RunGovard(t, projectDir, "config", "set", "unknown.key", "x")
	unknown.AssertExitCode(t, 1)
	assertContains(t, strings.ToLower(unknown.Stdout+unknown.Stderr), "unknown config key: unknown.key")
}

func TestExtensionsAndCustomCommandsLifecycle(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "extensions-custom-m2")

	initResult := env.RunGovard(t, projectDir, "extensions", "init")
	initResult.AssertSuccess(t)
	assertContains(t, initResult.Stdout+initResult.Stderr, "Extension contract scaffolded")

	requiredFiles := []string{
		filepath.Join(projectDir, ".govard", "govard.local.yml"),
		filepath.Join(projectDir, ".govard", "docker-compose.override.yml"),
		filepath.Join(projectDir, ".govard", "hooks", "pre_up.sh"),
		filepath.Join(projectDir, ".govard", "commands", "hello"),
	}
	for _, path := range requiredFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected scaffold file %s: %v", path, err)
		}
	}

	listResult := env.RunGovard(t, projectDir, "custom", "list")
	listResult.AssertSuccess(t)
	assertContains(t, listResult.Stdout+listResult.Stderr, "hello")

	helloResult := env.RunGovard(t, projectDir, "custom", "hello", "one", "two")
	helloResult.AssertSuccess(t)
	assertContains(t, helloResult.Stdout, "Hello from .govard/commands/hello")
	assertContains(t, helloResult.Stdout, "Args: one two")

	fallbackPath := filepath.Join(projectDir, ".govard", "commands", "fallback")
	fallbackScript := "#!/usr/bin/env bash\nset -euo pipefail\necho \"fallback:$*\"\n"
	if err := os.WriteFile(fallbackPath, []byte(fallbackScript), 0o644); err != nil {
		t.Fatalf("failed to write fallback script: %v", err)
	}
	fallbackResult := env.RunGovard(t, projectDir, "custom", "fallback", "alpha")
	fallbackResult.AssertSuccess(t)
	assertContains(t, fallbackResult.Stdout, "fallback:alpha")

	helloPath := filepath.Join(projectDir, ".govard", "commands", "hello")
	customHello := "#!/usr/bin/env bash\nset -euo pipefail\necho 'custom hello preserved'\n"
	if err := os.WriteFile(helloPath, []byte(customHello), 0o755); err != nil {
		t.Fatalf("failed to customize hello command: %v", err)
	}

	withoutForce := env.RunGovard(t, projectDir, "extensions", "init")
	withoutForce.AssertSuccess(t)
	assertContains(t, withoutForce.Stdout+withoutForce.Stderr, "No files changed")

	helloContent, err := os.ReadFile(helloPath)
	if err != nil {
		t.Fatalf("failed to read hello script: %v", err)
	}
	if !strings.Contains(string(helloContent), "custom hello preserved") {
		t.Fatalf("expected custom hello script to be preserved without --force, got:\n%s", string(helloContent))
	}

	withForce := env.RunGovard(t, projectDir, "extensions", "init", "--force")
	withForce.AssertSuccess(t)
	assertContains(t, withForce.Stdout+withForce.Stderr, "Extension contract scaffolded")

	forcedHelloContent, err := os.ReadFile(helloPath)
	if err != nil {
		t.Fatalf("failed to read hello script after --force: %v", err)
	}
	assertContains(t, string(forcedHelloContent), "Hello from .govard/commands/hello")
}

func TestSvcCommandsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "proxy-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	upResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "svc", "up")
	upResult.AssertSuccess(t)

	downResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "svc", "down")
	downResult.AssertSuccess(t)

	restartResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "svc", "restart")
	restartResult.AssertSuccess(t)

	psResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "svc", "ps")
	psResult.AssertSuccess(t)

	logsResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "svc", "logs")
	logsResult.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|compose --project-directory")
	assertContains(t, logs, " -p proxy ")
	assertContains(t, logs, " up -d")
	assertContains(t, logs, " down")
	assertContains(t, logs, " ps")
	assertContains(t, logs, " logs -f --tail=100")
}
