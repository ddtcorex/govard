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

func TestBootstrapOptionsMatrixWithSimulatedEnvironments(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("CloneCodeOnlyOption", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-options-code-only")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), isolatedHomeEnv(t)...),
			"bootstrap",
			"--clone",
			"--code-only",
			"--environment", "dev",
			"--no-up",
			"--no-composer",
			"--no-admin",
			"--yes",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "deploy@dev.example.com:'/var/www/html/'")
		if got := strings.Count(logs, "rsync|"); got != 1 {
			t.Fatalf("expected one rsync invocation for --code-only, got %d logs:\n%s", got, logs)
		}
		assertMatrixNotContains(t, logs, "MYSQL_PWD=magento")
	})

	t.Run("CloneNoStreamDBFromStaging", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-options-no-stream-db")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), isolatedHomeEnv(t)...),
			"bootstrap",
			"--clone",
			"--environment", "staging",
			"--no-up",
			"--no-composer",
			"--no-media",
			"--no-admin",
			"--no-stream-db",
			"--yes",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "deploy@staging.example.com:'/srv/www/staging/'")
		assertMatrixContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 sh -lc if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb;")
		assertMatrixNotContains(t, logs, "DROP DATABASE IF EXISTS")
	})

	t.Run("CloneDbDumpOption", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-options-db-dump")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})
		dumpPath := filepath.Join(projectDir, "fixtures", "bootstrap-dump.sql")
		if err := os.MkdirAll(filepath.Dir(dumpPath), 0o755); err != nil {
			t.Fatalf("create dump dir: %v", err)
		}
		if err := os.WriteFile(dumpPath, []byte("CREATE TABLE test_options(id INT);\n"), 0o644); err != nil {
			t.Fatalf("write dump file: %v", err)
		}

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), isolatedHomeEnv(t)...),
			"bootstrap",
			"--clone",
			"--environment", "dev",
			"--no-up",
			"--no-composer",
			"--no-media",
			"--no-admin",
			"--db-dump", "fixtures/bootstrap-dump.sql",
			"--yes",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 sh -lc if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb;")
		assertMatrixNotContains(t, logs, "DROP DATABASE IF EXISTS")
	})

	t.Run("FreshInstallCanonicalOptions", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "bootstrap-options-fresh-canonical")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})

		homeVars := isolatedHomeEnv(t)
		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), homeVars...),
			"bootstrap",
			"--fresh",
			"--no-up",
			"--meta-package", "magento/project-enterprise-edition",
			"--framework-version", "2.4.8",
			"--include-sample",
			"--hyva-install",
			"--hyva-token", "test-hyva-token",
			"--mage-username", "mage-user",
			"--mage-password", "mage-pass",
			"--yes",
			"--no-admin",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "magento/project-enterprise-edition")
		assertMatrixContains(t, logs, "2.4.8")
		assertMatrixContains(t, logs, "hyva-themes.repo.packagist.com")
		assertMatrixContains(t, logs, "test-hyva-token")
		assertMatrixContains(t, logs, "sample:deploy")
		assertMatrixNotContains(t, logs, "admin:user:create")

		// The new logic saves to global ~/.composer/auth.json (host)
		homeDir := ""
		for _, envVar := range homeVars {
			if strings.HasPrefix(envVar, "HOME=") {
				homeDir = strings.TrimPrefix(envVar, "HOME=")
			}
		}

		authPath := filepath.Join(homeDir, ".composer", "auth.json")
		data, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatalf("expected auth.json generated from mage credentials in %s: %v", authPath, err)
		}
		assertMatrixContains(t, string(data), "\"username\": \"mage-user\"")
		assertMatrixContains(t, string(data), "\"password\": \"mage-pass\"")
	})

	t.Run("InitFrameworkAndFrameworkVersionWhenConfigMissing", func(t *testing.T) {
		projectDir := env.CreateTestProject(t, "bootstrap-options-init", map[string]string{
			"composer.json": `{"name":"test/bootstrap-options-init"}`,
		})

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			isolatedHomeEnv(t),
			"bootstrap",
			"--clone=false",
			"--no-up",
			"--no-db",
			"--no-media",
			"--no-composer",
			"--no-admin",
			"--framework", "magento2",
			"--framework-version", "2.4.8",
			"--yes",
		)
		result.AssertSuccess(t)

		cfg, err := engine.LoadBaseConfigFromDir(projectDir, true)
		if err != nil {
			t.Fatalf("load generated govard config: %v", err)
		}
		if cfg.Framework != "magento2" {
			t.Fatalf("expected framework magento2, got %s", cfg.Framework)
		}
		if cfg.FrameworkVersion != "2.4.8" {
			t.Fatalf("expected framework version 2.4.8, got %s", cfg.FrameworkVersion)
		}
	})
}

func TestSyncOptionsMatrixWithSimulatedEnvironments(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("PlanDefaultsToStagingLocalAndFileScope", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-default-plan")
		result := env.RunGovardWithEnv(
			t,
			projectDir,
			isolatedHomeEnv(t),
			"sync",
			"--plan",
		)
		result.AssertSuccess(t)

		out := result.Stdout
		assertMatrixContains(t, out, "Source:      staging (Target: deploy@staging.example.com")
		assertMatrixContains(t, out, "Destination: local (local project:")
		assertMatrixContains(t, out, "Scopes:      files")
		assertMatrixContains(t, out, "Resume Mode: Enabled")
		assertMatrixContains(t, out, "Planned Actions:")
	})

	t.Run("PlanFullDeletePathIncludeExcludeResume", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-full-plan")
		result := env.RunGovardWithEnv(
			t,
			projectDir,
			isolatedHomeEnv(t),
			"sync",
			"--source", "staging",
			"--destination", "local",
			"--full",
			"--delete",
			"--resume",
			"--path", "app/code",
			"--include", "app/*",
			"--exclude", "vendor/",
			"--plan",
		)
		result.AssertSuccess(t)

		out := result.Stdout
		assertMatrixContains(t, out, "Scopes:      files, media (all), db")
		assertMatrixContains(t, out, "Path Filter: app/code")
		assertMatrixContains(t, out, "Includes:    app/*")
		assertMatrixContains(t, out, "Excludes:    vendor/")
		assertMatrixContains(t, out, "Resume Mode: Enabled")
		assertMatrixContains(t, out, "Delete Mode: Enabled")
		assertMatrixContains(t, out, "Risk Level:")
		assertMatrixContains(t, out, "HIGH RISK")
		assertMatrixContains(t, out, "Path filter only applies to file synchronization; media and database will use full configured paths.")
		assertMatrixContains(t, out, "--delete")
		assertMatrixContains(t, out, "--include app/*")
		assertMatrixContains(t, out, "--exclude vendor/")
	})

	t.Run("PlanNoResume", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-no-resume")
		result := env.RunGovardWithEnv(
			t,
			projectDir,
			isolatedHomeEnv(t),
			"sync",
			"--source", "dev",
			"--destination", "local",
			"--file",
			"--no-resume",
			"--plan",
		)
		result.AssertSuccess(t)

		out := result.Stdout
		assertMatrixContains(t, out, "Resume Mode: Disabled")
		assertMatrixNotContains(t, out, "--append-verify")
	})

	t.Run("RuntimeLocalToStagingFileDeletePath", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-runtime-local-to-staging-file")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), isolatedHomeEnv(t)...),
			"sync",
			"--source", "local",
			"--destination", "staging",
			"--file",
			"--delete",
			"--path", "app/code",
			"--yes",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "rsync|")
		assertMatrixContains(t, logs, "--delete")
		assertMatrixContains(t, logs, "deploy@staging.example.com:'/srv/www/staging/app/code/'")
		assertMatrixContains(t, logs, filepath.Join(projectDir, "app", "code"))
	})

	t.Run("RuntimeStagingToLocalMediaIncludeExcludeNoResume", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-runtime-media")
		shim := env.SetupRuntimeShims(t, map[string]int{
			"docker": 0,
			"ssh":    0,
			"rsync":  0,
		})

		result := env.RunGovardWithEnv(
			t,
			projectDir,
			append(shim.Env(), isolatedHomeEnv(t)...),
			"sync",
			"--source", "staging",
			"--destination", "local",
			"--media", "optimized",
			"--include", "catalog/*",
			"--exclude", "catalog/cache",
			"--no-resume",
			"--yes",
		)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertMatrixContains(t, logs, "deploy@staging.example.com:'/srv/www/staging/pub/media/'")
		assertMatrixContains(t, logs, "--include catalog/*")
		assertMatrixContains(t, logs, "--exclude catalog/cache")
		assertMatrixNotContains(t, logs, "--append-verify")
	})

	t.Run("RuntimeDatabaseBothDirections", func(t *testing.T) {
		t.Run("RemoteToLocal", func(t *testing.T) {
			projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-runtime-db-remote-local")
			shim := env.SetupRuntimeShims(t, map[string]int{
				"docker": 0,
				"ssh":    0,
				"rsync":  0,
			})

			result := env.RunGovardWithEnv(
				t,
				projectDir,
				append(shim.Env(), isolatedHomeEnv(t)...),
				"sync",
				"--source", "dev",
				"--destination", "local",
				"--db",
				"--yes",
			)
			result.AssertSuccess(t)

			logs := shim.ReadLog(t)
			assertMatrixContains(t, logs, "ssh|")
			assertMatrixContains(t, logs, "deploy@dev.example.com")
			assertMatrixContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 sh -lc if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb;")
		})

		t.Run("LocalToRemote", func(t *testing.T) {
			projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-runtime-db-local-remote")
			shim := env.SetupRuntimeShims(t, map[string]int{
				"docker": 0,
				"ssh":    0,
				"rsync":  0,
			})

			result := env.RunGovardWithEnv(
				t,
				projectDir,
				append(shim.Env(), isolatedHomeEnv(t)...),
				"sync",
				"--source", "local",
				"--destination", "staging",
				"--db",
				"--yes",
			)
			result.AssertSuccess(t)

			logs := shim.ReadLog(t)
			assertMatrixContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 sh -lc if command -v mariadb-dump")
			assertMatrixContains(t, logs, "ssh|")
			assertMatrixContains(t, logs, "deploy@staging.example.com")
		})

		t.Run("RemoteToLocalWithNoPII", func(t *testing.T) {
			projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "sync-options-runtime-db-remote-local-nopii")
			shim := env.SetupRuntimeShims(t, map[string]int{
				"docker": 0,
				"ssh":    0,
				"rsync":  0,
			})

			result := env.RunGovardWithEnv(
				t,
				projectDir,
				append(shim.Env(), isolatedHomeEnv(t)...),
				"sync",
				"--source", "dev",
				"--destination", "local",
				"--db",
				"--no-pii",
				"--yes",
			)
			result.AssertSuccess(t)

			logs := shim.ReadLog(t)
			assertMatrixContains(t, logs, "ssh|")
			assertMatrixContains(t, logs, "--ignore-table=magento.customer_entity")
		})
	})
}

func isolatedHomeEnv(t *testing.T) []string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated home dir: %v", err)
	}
	return []string{"HOME=" + home}
}

func assertMatrixContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q in output, got:\n%s", needle, haystack)
	}
}

func assertMatrixNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("did not expect %q in output, got:\n%s", needle, haystack)
	}
}
