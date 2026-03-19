package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestParseEnvMapForTest(t *testing.T) {
	raw := "MYSQL_USER=app\nMYSQL_PASSWORD=secret\nMYSQL_DATABASE=shop\nNO_VALUE\n"
	parsed := cmd.ParseEnvMapForTest(raw)
	if parsed["MYSQL_USER"] != "app" {
		t.Fatalf("expected MYSQL_USER=app, got %q", parsed["MYSQL_USER"])
	}
	if parsed["MYSQL_PASSWORD"] != "secret" {
		t.Fatalf("expected MYSQL_PASSWORD=secret, got %q", parsed["MYSQL_PASSWORD"])
	}
	if parsed["MYSQL_DATABASE"] != "shop" {
		t.Fatalf("expected MYSQL_DATABASE=shop, got %q", parsed["MYSQL_DATABASE"])
	}
	if _, ok := parsed["NO_VALUE"]; ok {
		t.Fatal("did not expect malformed env line to be parsed")
	}
}

func TestBuildRemoteMySQLDumpCommandForTest(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandForTest("db.internal", 3307, "remote_user", "s3cret", "remote_db", true)
	checks := []string{
		"export MYSQL_PWD='s3cret';",
		"mysqldump",
		"--max-allowed-packet=512M",
		"-h'db.internal'",
		"-P3307",
		"-u'remote_user'",
		"--routines",
		"--events",
		"--triggers",
		"'remote_db'",
	}
	for _, check := range checks {
		if !strings.Contains(command, check) {
			t.Fatalf("expected command to contain %q, got: %s", check, command)
		}
	}
}

func TestBuildRemoteMySQLDumpCommandWithoutHostPort(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandForTest("", 0, "remote_user", "", "remote_db", false)
	if strings.Contains(command, "-h") {
		t.Fatalf("did not expect host flag when host is empty: %s", command)
	}
	if strings.Contains(command, "-P") {
		t.Fatalf("did not expect port flag when port is zero: %s", command)
	}
	if strings.Contains(command, "MYSQL_PWD") {
		t.Fatalf("did not expect MYSQL_PWD export when password is empty: %s", command)
	}
	if !strings.Contains(command, "mysqldump") {
		t.Fatalf("expected mysqldump command: %s", command)
	}
}

func TestBuildLocalDBImportCommandSupportsMariaDBClient(t *testing.T) {
	args := cmd.BuildLocalDBImportCommandForTest("example-db-1", "etl", "secret", "etl_dev")
	if len(args) == 0 {
		t.Fatal("expected command args")
	}

	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"sh -lc",
		"command -v mysql",
		"command -v mariadb",
		"DB_CLI",
		"--max-allowed-packet=512M -u 'etl'",
		"'etl_dev' -f",
		"MYSQL_PWD=secret",
		"SET FOREIGN_KEY_CHECKS=0",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected import command to contain %q, got: %s", expected, joined)
		}
	}
}

func TestBuildLocalDBResetScriptSupportsMariaDBClient(t *testing.T) {
	script, err := cmd.BuildLocalDBResetScriptForTest("symfony")
	if err != nil {
		t.Fatalf("expected valid db reset script, got error: %v", err)
	}

	for _, expected := range []string{
		"command -v mysql",
		"command -v mariadb",
		"DB_CLI",
		"DROP DATABASE IF EXISTS `symfony`; CREATE DATABASE `symfony`;",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("expected reset script to contain %q, got: %s", expected, script)
		}
	}
}

func TestBuildLocalDBResetScriptRejectsUnsafeDatabaseName(t *testing.T) {
	_, err := cmd.BuildLocalDBResetScriptForTest("symfony; DROP DATABASE mysql;")
	if err == nil {
		t.Fatal("expected unsafe database name to be rejected")
	}

	if !strings.Contains(err.Error(), "invalid database name") {
		t.Fatalf("expected invalid database name error, got: %v", err)
	}
}
