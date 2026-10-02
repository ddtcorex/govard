package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestProcessListCommandKeepsPasswordOutOfArgv(t *testing.T) {
	const password = "s3cr3t pw'x"
	command := cmd.BuildProcessListCommandForTest("app", password, "SHOW FULL PROCESSLIST")

	if !strings.HasPrefix(command, "export MYSQL_PWD="+engine.ShellQuote(password)+"; ") {
		t.Fatalf("expected a MYSQL_PWD export prefix, got %q", command)
	}
	if !strings.Contains(command, "mysql --no-defaults -u'app' -BN") {
		t.Fatalf("expected --no-defaults and no -p argument, got %q", command)
	}
	rest := command[strings.Index(command, "mysql "):]
	if strings.Contains(rest, " -p") || strings.Contains(rest, "s3cr3t") {
		t.Fatalf("password must not appear in the mysql argv, got %q", rest)
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
