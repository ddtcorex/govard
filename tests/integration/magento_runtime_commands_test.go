//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBCommandValidationAndRuntime(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("ConnectRejectsStreamDB", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-validate-connect")
		result := env.RunGovard(t, projectDir, "db", "connect", "--stream-db")
		if result.Success() {
			t.Fatal("expected db connect --stream-db to fail")
		}
		assertContains(t, result.Stderr, "connect does not support --file, --stream-db, --full, or --exclude-sensitive-data")
	})

	t.Run("ImportStreamDBRequiresRemoteEnv", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-validate-import")
		result := env.RunGovard(t, projectDir, "db", "import", "--stream-db")
		if result.Success() {
			t.Fatal("expected db import --stream-db without remote environment to fail")
		}
		assertContains(t, result.Stderr, "--stream-db requires a remote --environment source")
	})

	t.Run("RemoteDumpUsesSSHShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-remote-dump")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "dump", "--environment", "dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		assertContains(t, logs, "mysqldump")
	})

	t.Run("RemoteImportFromFileUsesSSHShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-remote-import")
		dumpPath := filepath.Join(projectDir, "dump.sql")
		if err := os.WriteFile(dumpPath, []byte("CREATE TABLE t (id INT);\n"), 0o644); err != nil {
			t.Fatalf("failed to write dump file: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "import", "--environment", "dev", "--file", dumpPath)
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		assertContains(t, logs, "mysql")
	})

	t.Run("LocalDumpUsesDockerShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-local-dump")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "dump", "--environment", "local")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|inspect -f {{.State.Running}} m2-clone-basic-db-1")
		assertContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 mysqldump")
	})
}

func TestRemoteCommandRuntimeWithShims(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("RemoteTestUsesSSHChecks", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "remote-test-shims")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "remote", "test", "dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		assertContains(t, logs, "govard-remote-ok")
		assertContains(t, logs, "govard-rsync-ok")
	})

	t.Run("RemoteExecUsesSSH", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "remote-exec-shims")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "remote", "exec", "dev", "--", "pwd")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		assertContains(t, logs, "pwd")
	})
}

func TestConfigureMagentoWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "configure-m2-shims")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "config", "auto")
	result.AssertSuccess(t)

	output := strings.ToLower(result.Stdout + result.Stderr)
	assertContains(t, output, "auto-configuration")
}

func TestDeployHooksExecute(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "deploy-hooks")

	localOverride := `hooks:
  pre_deploy:
    - name: pre deploy marker
      run: "echo pre >> .govard-deploy-hooks.log"
  post_deploy:
    - name: post deploy marker
      run: "echo post >> .govard-deploy-hooks.log"
`
	overridePath := filepath.Join(projectDir, ".govard.local.yml")
	if err := os.WriteFile(overridePath, []byte(localOverride), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovard(t, projectDir, "deploy")
	result.AssertSuccess(t)

	content, err := os.ReadFile(filepath.Join(projectDir, ".govard-deploy-hooks.log"))
	if err != nil {
		t.Fatalf("failed to read deploy hook log: %v", err)
	}
	out := strings.TrimSpace(string(content))
	if out != "pre\npost" {
		t.Fatalf("expected deploy hook order pre->post, got:\n%s", out)
	}
}
