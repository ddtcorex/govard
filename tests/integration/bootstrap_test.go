//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
)

func TestBootstrapValidationMatrix(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("FreshAndCloneMutuallyExclusive", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-dev", "bootstrap-validate-fresh-clone")
		result := env.RunGovard(t, projectDir, "bootstrap", "--fresh", "--clone", "--no-up", "--yes")
		assertBootstrapContains(t, result.Stdout+result.Stderr, "--fresh and --clone cannot be used together")
	})

	t.Run("CodeOnlyRequiresClone", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-dev", "bootstrap-validate-code-only")
		result := env.RunGovard(t, projectDir, "bootstrap", "--code-only", "--no-up", "--yes")
		assertBootstrapContains(t, result.Stdout+result.Stderr, "--code-only requires --clone")
	})

	t.Run("InvalidVersionRejected", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-dev", "bootstrap-validate-version")
		result := env.RunGovard(t, projectDir, "bootstrap", "--fresh", "--framework-version", "1.0.0", "--no-up", "--yes")
		assertBootstrapContains(t, result.Stdout+result.Stderr, "invalid --framework-version value")
	})
}

func TestBootstrapCloneRequiresConfiguredRemote(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-dev", "bootstrap-no-remote")

	result := env.RunGovard(t, projectDir, "bootstrap", "--clone", "--environment", "dev", "--no-up", "--yes")
	assertBootstrapContains(t, result.Stdout+result.Stderr, "remote 'dev' is not configured")
}

func TestBootstrapCloneOrchestrationWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-clone-shims")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"bootstrap",
		"--clone", "--yes",

		"--environment", "dev",
		"--no-up",
		"--no-composer",
		"--no-db",
		"--no-media",
		"--no-admin",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertBootstrapContains(t, logs, "ssh|")
	assertBootstrapContains(t, logs, "govard-remote-ok")
	assertBootstrapContains(t, logs, "rsync|")
	assertBootstrapContains(t, logs, "deploy@dev.example.com:'/var/www/html/'")
}

func TestBootstrapFreshOrchestrationWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-dev", "bootstrap-fresh-shims")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"bootstrap",
		"--fresh", "--yes",
		"--no-up",

		"--no-admin",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertBootstrapContains(t, logs, "docker|exec")
	assertBootstrapContains(t, logs, "/tmp/govard-create-project")
	// The runtime PHP flows from the project config into the fresh install,
	// which pins composer's platform before resolving dependencies.
	assertBootstrapContains(t, logs, "create-project -n --no-install")
	assertBootstrapContains(t, logs, "config platform.php '")
	assertBootstrapContains(t, logs, "command -v rsync")
	assertBootstrapContains(t, logs, "setup:install")
}

func TestBootstrapCloneMediaSyncExcludesProductByDefault(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-media-default")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"bootstrap",
		"--clone", "--yes",

		"--environment", "dev",
		"--no-up",
		"--no-composer",
		"--no-db",
		"--no-admin",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertBootstrapContains(t, logs, "--exclude catalog/product")
}

func TestBootstrapCloneMediaSyncAllMode(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-media-all")
	shim := env.SetupRuntimeShims(t, map[string]int{
		"docker": 0,
		"ssh":    0,
		"rsync":  0,
	})

	result := env.RunGovardWithEnv(
		t,
		projectDir,
		shim.Env(),
		"bootstrap",
		"--clone", "--yes",

		"--environment", "dev",
		"--media", "all",
		"--no-up",
		"--no-composer",
		"--no-db",
		"--no-admin",
	)
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	if strings.Contains(logs, "--exclude catalog/product") {
		t.Fatalf("did not expect catalog/product exclusion when --media all is set, got:\n%s", logs)
	}
	if strings.Contains(logs, "--exclude *cache*/") {
		t.Fatalf("expected all sync patterns to be included (no cache exclude) when --media all is set, but got exclude:\n%s", logs)
	}
}

func assertBootstrapContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q in output, got:\n%s", needle, haystack)
	}
}
