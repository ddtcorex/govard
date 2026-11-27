//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
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
		assertContains(t, result.Stdout+result.Stderr, "connect does not support --file, --stream-db, --no-noise, --no-pii, --drop, or --local")
	})

	t.Run("ImportStreamDBRequiresRemoteEnv", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-validate-import")
		result := env.RunGovard(t, projectDir, "db", "import", "--stream-db")
		if result.Success() {
			t.Fatal("expected db import --stream-db without remote environment to fail")
		}
		assertContains(t, result.Stdout+result.Stderr, "--stream-db requires a remote --environment source")
	})

	t.Run("RemoteDumpUsesSSHShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-remote-dump")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "dump", "--environment", "dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		// The dump script travels on stdin now, so the argv carries `sh -s` and the
		// credential and dump flags are deliberately absent from it.
		assertContains(t, logs, "sh -s")
		assertNotContains(t, logs, "mysqldump")
	})

	t.Run("RemoteImportFromFileUsesSSHShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-remote-import")
		dumpPath := filepath.Join(projectDir, "dump.sql")
		if err := os.WriteFile(dumpPath, []byte("CREATE TABLE t (id INT);\n"), 0o644); err != nil {
			t.Fatalf("failed to write dump file: %v", err)
		}

		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "import", "--environment", "dev", "--file", dumpPath, "--yes")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "ssh|")
		// stdin carries the SQL, so the credential is the first stdin line, read by
		// the remote shell into the client's environment: the argv names the
		// variable but never carries a value, and no script is staged on the remote.
		assertContains(t, logs, "read -r MYSQL_PWD")
		assertNotContains(t, logs, "MYSQL_PWD=")
		assertNotContains(t, logs, "/tmp/govard-")
	})

	t.Run("LocalDumpUsesDockerShim", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-runtime-local-dump")
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "dump", "--environment", "local")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "docker|inspect -f {{.State.Running}} m2-clone-basic-db-1")
		assertContains(t, logs, "docker|exec -i -e MYSQL_PWD m2-clone-basic-db-1 sh -lc if command -v mariadb-dump")
	})
}
