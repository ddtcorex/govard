package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/conventions"
	"govard/internal/engine"
)

func TestProcessListCommandKeepsPasswordOutOfArgv(t *testing.T) {
	const password = "s3cr3t pw'x"
	command := cmd.BuildProcessListCommandForTest("app", password, "SHOW FULL PROCESSLIST")

	if !strings.HasPrefix(command, "export MYSQL_PWD="+engine.ShellQuote(password)+"; ") {
		t.Fatalf("expected a MYSQL_PWD export prefix, got %q", command)
	}
	if !strings.Contains(command, `"$DB_CLI" --no-defaults -u'app' -BN`) {
		t.Fatalf("expected --no-defaults and no -p argument, got %q", command)
	}
	rest := command[strings.Index(command, `"$DB_CLI" `):]
	if strings.Contains(rest, " -p") || strings.Contains(rest, "s3cr3t") {
		t.Fatalf("password must not appear in the client argv, got %q", rest)
	}
}

func TestProcessListCommandWithoutPasswordHasNoExport(t *testing.T) {
	command := cmd.BuildProcessListCommandForTest("app", "", "SHOW FULL PROCESSLIST")
	if strings.Contains(command, "MYSQL_PWD") || strings.Contains(command, " -p") {
		t.Fatalf("expected no password handling, got %q", command)
	}
}

func TestDBReadinessProbeKeepsPasswordOutOfArgv(t *testing.T) {
	command := cmd.DBReadinessProbeScriptForTest("app", "s3cr3t")
	if !strings.Contains(command, "export MYSQL_PWD='s3cr3t'; ") {
		t.Fatalf("expected MYSQL_PWD export, got %q", command)
	}
	if strings.Contains(command, " -ps3cr3t") || strings.Contains(command, " -p'") {
		t.Fatalf("password must not be an argv argument, got %q", command)
	}
}

// MariaDB 11 images ship `mariadb` and no `mysql`: db top must find whichever
// client exists, like db query does, instead of looping on "mysql: not found".
func TestProcessListCommandDetectsTheClientBinary(t *testing.T) {
	command := cmd.BuildProcessListCommandForTest("app", "", "SHOW FULL PROCESSLIST")
	if !strings.Contains(command, conventions.MySQLClientBinDetect) {
		t.Fatalf("the processlist command must detect mysql or mariadb, got %q", command)
	}
	if !strings.Contains(command, `"$DB_CLI" --no-defaults`) {
		t.Fatalf("the processlist command must run the detected client, got %q", command)
	}
	if strings.Contains(command, "; mysql ") || strings.HasPrefix(command, "mysql ") {
		t.Fatalf("the processlist command must not hard-code mysql, got %q", command)
	}
}
