package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RunSQLViaDockerExec hands the database password to `docker exec -e MYSQL_PWD`
// by name, with the value in the docker CLI's own environment, so it is never
// in the argument list another user on the host can read.
func TestRunSQLViaDockerExecKeepsThePasswordOutOfArgv(t *testing.T) {
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	passwordFile := filepath.Join(dir, "password")
	shim := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SHIM_ARGV_FILE\"\nprintf '%s' \"$MYSQL_PWD\" > \"$SHIM_PASSWORD_FILE\"\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SHIM_ARGV_FILE", argvFile)
	t.Setenv("SHIM_PASSWORD_FILE", passwordFile)

	if err := RunSQLViaDockerExec("sample-project-db-1", "app", "s3cr3t-local", "app", "SELECT 1"); err != nil {
		t.Fatalf("RunSQLViaDockerExec: %v", err)
	}

	argv, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(argv), "s3cr3t-local") {
		t.Fatalf("the password is in the docker argument list: %q", argv)
	}
	if !strings.Contains(string(argv), "MYSQL_PWD") {
		t.Fatalf("the variable is not passed by name: %q", argv)
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(password) != "s3cr3t-local" {
		t.Fatalf("docker saw MYSQL_PWD=%q in its environment, want s3cr3t-local", password)
	}
}
