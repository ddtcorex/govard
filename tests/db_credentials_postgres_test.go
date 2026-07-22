package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestDefaultDBCredentialsForDjangoUsesPostgresEngine(t *testing.T) {
	creds := cmd.DefaultDBCredentialsForFrameworkForTest("django")
	if creds.Engine != "postgres" {
		t.Errorf("Engine = %q, want %q", creds.Engine, "postgres")
	}
	if creds.Port != 5432 {
		t.Errorf("Port = %d, want 5432", creds.Port)
	}
	if creds.Username != "django" || creds.Database != "django" {
		t.Errorf("Username/Database = %q/%q, want django/django", creds.Username, creds.Database)
	}
}

// TestDBEngineIsDerivedFromFrameworkConfigNotHardcoded locks in that Engine
// resolution is generic (keyed off FrameworkConfig.DefaultDB), not a
// django-only literal - a mariadb-default framework must NOT get "postgres".
func TestDBEngineIsDerivedFromFrameworkConfigNotHardcoded(t *testing.T) {
	creds := cmd.DefaultDBCredentialsForFrameworkForTest("laravel")
	if creds.Engine == "postgres" {
		t.Errorf("Engine = %q, want anything but postgres for a mariadb-default framework", creds.Engine)
	}
}

func TestBuildRemoteDBDumpCommandForDjangoUsesPgDump(t *testing.T) {
	command := cmd.BuildRemoteDBDumpCommandForFrameworkForTest("django", "remote-host", 5432, "django", "secret", "django", false)
	if !strings.Contains(command, "pg_dump") {
		t.Fatalf("expected pg_dump in command, got: %s", command)
	}
	if !strings.Contains(command, "PGPASSWORD='secret'") {
		t.Fatalf("expected PGPASSWORD export, got: %s", command)
	}
	if strings.Contains(command, "mysqldump") {
		t.Fatalf("did not expect mysqldump for a postgres-engine dump, got: %s", command)
	}
}

func TestBuildLocalDBConnectCommandForDjangoUsesPsql(t *testing.T) {
	args := cmd.BuildLocalDBConnectCommandArgsForFrameworkForTest("django", "myproj-db-1", "django", "django")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "psql") {
		t.Fatalf("expected psql in connect command, got: %s", joined)
	}
	if strings.Contains(joined, "mysql") {
		t.Fatalf("did not expect mysql client for a postgres-engine connect, got: %s", joined)
	}
}

func TestBuildLocalDBResetScriptForDjangoUsesDropDatabaseNoQuotes(t *testing.T) {
	script, err := cmd.BuildLocalDBResetScriptForFrameworkForTest("django", "django")
	if err != nil {
		t.Fatalf("expected valid reset script, got error: %v", err)
	}
	if !strings.Contains(script, `DROP DATABASE IF EXISTS "django"`) {
		t.Fatalf("expected double-quoted Postgres DROP DATABASE, got: %s", script)
	}
	if strings.Contains(script, "`django`") {
		t.Fatalf("did not expect MySQL backtick-quoted identifiers, got: %s", script)
	}
}
