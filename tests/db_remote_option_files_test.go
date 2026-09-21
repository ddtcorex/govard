package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
)

// Remote mysql/mariadb clients must ignore option files (~/.my.cnf,
// /etc/mysql/*). Client password precedence is command-line > option file >
// MYSQL_PWD, so a stale ~/.my.cnf password on a shared-hosting remote would
// otherwise silently override the env.php password govard probed and exported
// via MYSQL_PWD (1045 "Access denied ... (using password: YES)" despite
// correct credentials — reproduced on a shared-hosting preprod, 2026-09-21).
func TestRemoteMySQLDumpIgnoresOptionFiles(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db", false)
	if !strings.Contains(command, `"$DUMP_BIN" --no-defaults`) {
		t.Fatalf("--no-defaults must be the first arg after the dump binary, got: %s", command)
	}
}

func TestRemoteMySQLConnectIgnoresOptionFiles(t *testing.T) {
	command := cmd.BuildRemoteMySQLConnectCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db")
	if !strings.Contains(command, `mysql --no-defaults`) {
		t.Fatalf("--no-defaults must be the first arg after the client binary, got: %s", command)
	}
}

func TestRemoteMySQLImportIgnoresOptionFiles(t *testing.T) {
	command := cmd.BuildRemoteMySQLImportCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db")
	if !strings.Contains(command, `mysql --no-defaults`) {
		t.Fatalf("--no-defaults must be the first arg after the client binary, got: %s", command)
	}
}

func TestRemoteMySQLQueryIgnoresOptionFiles(t *testing.T) {
	command := cmd.BuildRemoteMySQLQueryCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db", "SELECT 1")
	if !strings.Contains(command, `mysql --no-defaults`) {
		t.Fatalf("--no-defaults must be the first arg after the client binary, got: %s", command)
	}
}

func TestRemoteMySQLSizeProbeIgnoresOptionFiles(t *testing.T) {
	command := cmd.BuildRemoteMySQLSizeCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db")
	if !strings.Contains(command, `"$DB_CLI" --no-defaults`) {
		t.Fatalf("--no-defaults must be the first arg after the client binary, got: %s", command)
	}
}

func TestRemoteMySQLDumpStillExportsPassword(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db", false)
	if !strings.Contains(command, "export MYSQL_PWD='remote-pass';") {
		t.Fatalf("probed password must still reach the client via MYSQL_PWD, got: %s", command)
	}
}

func TestRemotePostgresDumpHasNoMySQLFlag(t *testing.T) {
	command := cmd.BuildRemoteDBDumpCommandForFrameworkForTest("django", "remote-host", 5432, "django", "secret", "django", false)
	if strings.Contains(command, "--no-defaults") {
		t.Fatalf("postgres path must not carry the mysql-only flag, got: %s", command)
	}
}

func TestLocalMySQLCommandsKeepOptionFiles(t *testing.T) {
	script := cmd.BuildLocalMySQLQueryCommandScriptForTest("user", "db", "SELECT 1")
	if strings.Contains(script, "--no-defaults") {
		t.Fatalf("local container commands must not skip option files, got: %s", script)
	}
}
