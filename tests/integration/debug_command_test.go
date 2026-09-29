//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugStatusAndShellDisabled(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "debug-m2")

	statusResult := env.RunGovard(t, projectDir, "debug", "status")
	statusResult.AssertSuccess(t)
	assertContains(t, strings.ToLower(statusResult.Stdout+statusResult.Stderr), "xdebug is currently")

	shellResult := env.RunGovard(t, projectDir, "debug", "shell")
	shellResult.AssertExitCode(t, 1)
	assertContains(t, strings.ToLower(shellResult.Stdout+shellResult.Stderr), "xdebug is disabled")
}

// A bare `govard debug` is an interactive session, so stdin has to reach the
// container: `docker exec` without -i hands bash an already-closed stdin and it
// exits on the spot, which is what made the command stop opening a shell.
func TestDebugShellKeepsStdinAttached(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "debug-shell-m2")

	overridePath := filepath.Join(projectDir, ".govard.local.yml")
	if err := os.WriteFile(overridePath, []byte("stack:\n  features:\n    xdebug: true\n"), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "debug")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|exec -i ")
	assertContains(t, logs, "php-debug-1 bash -c")
}

// A passthrough invocation is a one-shot wrapper, so it keeps its stdin
// detached — the same rule `govard sh -c` follows.
func TestDebugShellPassthroughRunsDetached(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "debug-passthrough-m2")

	overridePath := filepath.Join(projectDir, ".govard.local.yml")
	if err := os.WriteFile(overridePath, []byte("stack:\n  features:\n    xdebug: true\n"), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "debug", "shell", "-c", "php -v")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|exec -u ")
	assertNotContains(t, logs, "docker|exec -i ")
}

func assertNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected log NOT to contain %q, got:\n%s", needle, haystack)
	}
}
