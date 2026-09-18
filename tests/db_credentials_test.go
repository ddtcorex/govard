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
	command := cmd.BuildRemoteMySQLDumpCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db", false)
	checks := []string{
		"export MYSQL_PWD='remote-pass';",
		"DUMP_BIN",
		"--max-allowed-packet=512M",
		"-u'remote-user'",
		"--routines",
		"--triggers",
		"'remote-db'",
		"--no-data",
		"--no-create-info",
	}
	for _, check := range checks {
		if !strings.Contains(command, check) {
			t.Fatalf("expected command to contain %q, got: %s", check, command)
		}
	}
}

func TestBuildRemoteMySQLDumpCommandPropagatesDumpFailure(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandForTest("remote-host", 3306, "remote-user", "remote-pass", "remote-db", true)
	for _, want := range []string{"mysqldump", "mktemp", "gzip -c", "(exit "} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected compressed dump command to contain %q, got: %s", want, command)
		}
	}
	if strings.Contains(command, "| gzip") {
		t.Fatalf("compressed dump must not pipe mysqldump straight into gzip (masks failures), got: %s", command)
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

func TestBuildIgnoredTableArgsNoFlags(t *testing.T) {
	args := cmd.BuildIgnoredTableArgsForTest("magento", "", false, false, "magento2")
	if len(args) != 0 {
		t.Fatalf("expected no ignore-table args when no flags set, got %d args", len(args))
	}
}

func TestBuildIgnoredTableArgsNoNoise(t *testing.T) {
	args := cmd.BuildIgnoredTableArgsForTest("magento", "", true, false, "magento2")
	if len(args) == 0 {
		t.Fatal("expected ignore-table args when --no-noise is set")
	}
	joined := strings.Join(args, " ")
	// cron_schedule is an archetypal noise table
	if !strings.Contains(joined, "--ignore-table=magento.cron_schedule") {
		t.Fatalf("expected cron_schedule to be in ignore-table args, got: %s", joined)
	}
	// PII table (sales_order) should NOT appear when only --no-noise
	if strings.Contains(joined, "--ignore-table=magento.sales_order ") {
		t.Fatalf("did not expect sales_order in --no-noise only args, got: %s", joined)
	}
}

func TestBuildIgnoredTableArgsNoPII(t *testing.T) {
	args := cmd.BuildIgnoredTableArgsForTest("mydb", "", true, true, "magento2")
	if len(args) == 0 {
		t.Fatal("expected ignore-table args when --no-pii is set")
	}
	joined := strings.Join(args, " ")
	// Both noise table and PII table should appear
	if !strings.Contains(joined, "--ignore-table=mydb.cron_schedule") {
		t.Fatalf("expected noise table cron_schedule in --no-pii args, got: %s", joined)
	}
	if !strings.Contains(joined, "--ignore-table=mydb.customer_entity") {
		t.Fatalf("expected PII table customer_entity in --no-pii args, got: %s", joined)
	}
	if !strings.Contains(joined, "--ignore-table=mydb.sales_order") {
		t.Fatalf("expected PII table sales_order in --no-pii args, got: %s", joined)
	}
}

func TestBuildIgnoredTableArgsWithPrefix(t *testing.T) {
	args := cmd.BuildIgnoredTableArgsForTest("mage", "m2_", true, false, "magento2")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--ignore-table=mage.m2_cron_schedule") {
		t.Fatalf("expected prefixed table name, got: %s", joined)
	}
}

func TestBuildRemoteMySQLDumpCommandWithPrefixUsesPrefixedIgnoredTables(t *testing.T) {
	command := cmd.BuildRemoteMySQLDumpCommandWithPrefixForTest("mage", "demo_", true, false, "magento2")

	if !strings.Contains(command, "--ignore-table=mage.demo_cron_schedule") {
		t.Fatalf("expected remote dump command to use prefixed ignored tables, got: %s", command)
	}
}
func TestBuildLocalMySQLQueryCommandScriptInjection(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"Single quote injection", "'; DROP TABLE users; --"},
		{"Subshell injection", "$(whoami)"},
		{"Backtick injection", "`id`"},
		{"Command separator", "SELECT 1; cat /etc/passwd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := cmd.BuildLocalMySQLQueryCommandScriptForTest("user", "db", tc.query)

			// We should verify that the query is present but properly quoted/escaped.
			// Using engine.ShellQuote(tc.query) is the expected secure way.
			// We need to access ShellQuote, but it's in package engine.
			// However, we can just check if it matches the known safe pattern.

			expectedQuoted := "'" + strings.ReplaceAll(tc.query, "'", `'"'"'`) + "'"
			if !strings.Contains(script, " -e "+expectedQuoted) {
				t.Fatalf("vulnerability or bug: script does not contain properly quoted query. Expected to find %q in %s", expectedQuoted, script)
			}
		})
	}
}
