//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
)

func TestSyncPlanMagento2CurrentFlags(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-plan-current")

	result := env.RunGovard(
		t,
		projectDir,
		"sync",
		"--source", "dev",
		"--destination", "local",
		"--plan",
		"--file",
		"--include", "app/*",
		"--exclude", "vendor/",
	)
	result.AssertSuccess(t)

	out := result.Stdout
	assertContains(t, out, "Synchronization Plan Review")
	assertContains(t, out, "Scopes:      files")
	assertContains(t, out, "Includes:    app/*")
	assertContains(t, out, "Excludes:    vendor/")
	assertContains(t, out, "Resume Mode: Enabled")
	assertContains(t, out, "--partial --append-verify")
	assertContains(t, out, "--include app/*")
	assertContains(t, out, "--exclude vendor/")
}

func TestSyncPlanNoResume(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-plan-no-resume")

	result := env.RunGovard(
		t,
		projectDir,
		"sync",
		"--source", "dev",
		"--destination", "local",
		"--plan",
		"--file",
		"--no-resume",
	)
	result.AssertSuccess(t)

	out := result.Stdout
	assertContains(t, out, "Resume Mode: Disabled")
	if strings.Contains(out, "--append-verify") {
		t.Fatalf("did not expect resume rsync flags when --no-resume is set, got: %s", out)
	}
}

func TestSyncPolicyProtectedDestinationBlocked(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-staging", "sync-policy-protected")

	result := env.RunGovard(t, projectDir, "sync", "--source", "local", "--destination", "prod", "--file", "--yes")
	if result.Success() {
		t.Fatal("expected protected destination error")
	}
	assertContains(t, result.Stdout+result.Stderr, "Write-protected")
}

func TestSyncPolicyRemoteToRemoteBlocked(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-staging", "sync-policy-remote-remote")

	result := env.RunGovard(t, projectDir, "sync", "--source", "staging", "--destination", "prod", "--file", "--yes")
	if result.Success() {
		t.Fatal("expected local<->remote validation error")
	}
	assertContains(t, result.Stdout+result.Stderr, "between local and remote environments")
}

func TestSyncRuntimeRsyncInvocationWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-runtime-rsync")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"sync",
		"--source", "dev",
		"--destination", "local",
		"--file",
		"--yes",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "rsync|")
	assertContains(t, logs, "deploy@dev.example.com:'/var/www/html/'")
	assertContains(t, logs, "--partial --append-verify")
}

func TestSyncRuntimeDBPipelineWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-runtime-db")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"sync",
		"--source", "dev",
		"--destination", "local",
		"--db",
		"--yes",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "ssh|")
	assertContains(t, logs, "docker|exec -i -e MYSQL_PWD m2-clone-basic-db-1 sh -lc if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb;")
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q in output, got:\n%s", needle, haystack)
	}
}
