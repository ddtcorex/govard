package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// handoffShims puts an `ssh` that runs its last argument locally (the remote
// command line) and a fake database client that records what it was given on
// PATH. It returns the files the fake client writes.
func handoffShims(t *testing.T) (sqlFile string, passwordFile string) {
	t.Helper()
	dir := t.TempDir()
	sqlFile = filepath.Join(dir, "client-stdin")
	passwordFile = filepath.Join(dir, "client-password")

	sshShim := "#!/bin/sh\nfor a; do last=\"$a\"; done\nexec sh -c \"$last\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(sshShim), 0o755); err != nil {
		t.Fatal(err)
	}
	client := "#!/bin/sh\nprintf '%s' \"$MYSQL_PWD$PGPASSWORD\" > \"$HANDOFF_PASSWORD_FILE\"\ncat > \"$HANDOFF_SQL_FILE\"\n"
	for _, name := range []string{"mysql", "mariadb", "psql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(client), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("HANDOFF_SQL_FILE", sqlFile)
	t.Setenv("HANDOFF_PASSWORD_FILE", passwordFile)
	return sqlFile, passwordFile
}

func handoffRemote() engine.RemoteConfig {
	return engine.RemoteConfig{Host: "example.invalid", User: "deploy"}
}

func assertNoSecretInArgs(t *testing.T, args []string, secret string) {
	t.Helper()
	for _, arg := range args {
		if strings.Contains(arg, secret) {
			t.Fatalf("argument %q carries the password", arg)
		}
	}
}

// The remote import owns stdin for the SQL, so the credential has to reach the
// client some other way. This runs the whole path: the SQL must arrive, the
// password must arrive as the client's environment variable and never as SQL,
// and neither the ssh argv nor a file on the remote may hold it.
func TestRemoteImportDeliversTheSQLAndTheCredentialSeparately(t *testing.T) {
	sqlFile, passwordFile := handoffShims(t)
	credentials := dbCredentials{Engine: "mariadb", Host: "db.internal", Port: 3306, Username: "app", Password: "s3cr3t-pw", Database: "app"}

	command, secret, err := remoteImportCommand(credentials)
	if err != nil {
		t.Fatalf("remoteImportCommand: %v", err)
	}
	importCmd := remote.BuildSSHExecCommand("dev", handoffRemote(), true, command)
	importCmd.Stdin = strings.NewReader(secret)
	assertNoSecretInArgs(t, importCmd.Args, "s3cr3t-pw")

	var stderr bytes.Buffer
	sql := "CREATE TABLE handoff_t (id int);\nINSERT INTO handoff_t VALUES (1);\n"
	if err := RunImportFromReaderWithProgress(importCmd, strings.NewReader(sql), 0, false, io.Discard, &stderr, nil); err != nil {
		t.Fatalf("import failed: %v (%s)", err, stderr.String())
	}

	received, err := os.ReadFile(sqlFile)
	if err != nil {
		t.Fatalf("the database client never ran: %v", err)
	}
	if !strings.Contains(string(received), "CREATE TABLE handoff_t") || !strings.Contains(string(received), "INSERT INTO handoff_t") {
		t.Fatalf("the SQL did not reach the client: %q", received)
	}
	if strings.Contains(string(received), "s3cr3t-pw") {
		t.Fatalf("the credential line leaked into the SQL stream: %q", received)
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(password) != "s3cr3t-pw" {
		t.Fatalf("client saw password %q, want s3cr3t-pw", password)
	}
}

func TestRemoteImportCommandForPostgresUsesPGPASSWORD(t *testing.T) {
	sqlFile, passwordFile := handoffShims(t)
	credentials := dbCredentials{Engine: conventions.ServicePostgreSQL, Host: "db.internal", Port: 5432, Username: "app", Password: "pg-secret", Database: "app"}

	command, secret, err := remoteImportCommand(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "PGPASSWORD") || strings.Contains(command, "MYSQL_PWD") {
		t.Fatalf("postgres import must hand over PGPASSWORD, got %s", command)
	}
	importCmd := remote.BuildSSHExecCommand("dev", handoffRemote(), true, command)
	importCmd.Stdin = strings.NewReader(secret)
	if err := RunImportFromReaderWithProgress(importCmd, strings.NewReader("SELECT 1;\n"), 0, false, io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(passwordFile); string(got) != "pg-secret" {
		t.Fatalf("client saw password %q", got)
	}
	if got, _ := os.ReadFile(sqlFile); !strings.Contains(string(got), "SELECT 1;") {
		t.Fatalf("SQL missing: %q", got)
	}
}

func TestRemoteImportCommandWithoutPasswordSendsNoCredentialLine(t *testing.T) {
	command, secret, err := remoteImportCommand(dbCredentials{Engine: "mariadb", Username: "app", Database: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if secret != "" || strings.Contains(command, "read -r") {
		t.Fatalf("no password means no credential line, got secret %q command %s", secret, command)
	}
}

// A newline would end the credential line early and turn the rest of the
// password into SQL, so it is refused instead of sent.
func TestRemoteImportCommandRefusesAPasswordWithANewline(t *testing.T) {
	_, _, err := remoteImportCommand(dbCredentials{Engine: "mariadb", Username: "app", Password: "a\nb", Database: "app"})
	if err == nil {
		t.Fatal("a password containing a newline must be refused")
	}
}

// The connect session owns the terminal, so its script is staged on the remote.
// The staged file must hold the whole script: an empty one runs nothing and the
// command still exits 0.
func TestStageRemoteScriptWritesTheWholeScript(t *testing.T) {
	handoffShims(t)
	script := "export MYSQL_PWD='s3cr3t'; echo \"$HOME\" > /dev/null; echo STAGED_SCRIPT_RAN\n"

	path, err := stageRemoteScript("dev", handoffRemote(), script)
	if err != nil {
		t.Fatalf("stageRemoteScript: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(string(got), "\n") != strings.TrimRight(script, "\n") {
		t.Fatalf("staged file %q does not hold the script %q", got, script)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("staged file must be 0600, got %v (%v)", info, err)
	}

	runCmd := remote.BuildSSHExecCommand("dev", handoffRemote(), true, remoteScriptCleanup(path))
	out, err := runCmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "STAGED_SCRIPT_RAN") {
		t.Fatalf("the staged script did not run: err=%v out=%q", err, out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the cleanup tail left the staged script behind (%v)", err)
	}
}

func TestStageRemoteScriptRefusesAScriptThatContainsTheDelimiter(t *testing.T) {
	handoffShims(t)
	_, err := stageRemoteScript("dev", handoffRemote(), "echo hi\n"+stagedScriptDelimiter+"\nrm -rf /\n")
	if err == nil {
		t.Fatal("a script containing the heredoc delimiter could end the staging early and run its tail")
	}
}

// If the connection that would run the staged script never happens, the file
// holding the credential must not be left on the remote.
func TestDiscardRemoteScriptRemovesAStagedFile(t *testing.T) {
	handoffShims(t)
	path, err := stageRemoteScript("dev", handoffRemote(), "echo x\n")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	discardRemoteScript("dev", handoffRemote(), path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the staged script is still there: %v", err)
	}
}

// The builder wires the pieces together: the ssh command it returns carries the
// credential line as its stdin prefix and nothing secret in argv.
func TestBuildDBImportCommandForARemoteCarriesTheCredentialAsStdinPrefix(t *testing.T) {
	handoffShims(t)
	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "unknown",
		Remotes: map[string]engine.RemoteConfig{"dev": {
			Host: "example.invalid", User: "deploy",
			Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(true)},
		}},
	}
	importCmd, _, err := buildDBImportCommand(config, dbCommandOptions{Environment: "dev", File: "backup.sql", NoNoise: true})
	if err != nil {
		t.Fatalf("buildDBImportCommand: %v", err)
	}
	if importCmd.Stdin == nil {
		t.Fatal("the credential line was not attached as the command's stdin prefix")
	}
	prefix, _ := io.ReadAll(importCmd.Stdin)
	if !strings.HasSuffix(string(prefix), "\n") || strings.Count(string(prefix), "\n") != 1 {
		t.Fatalf("the prefix must be exactly one line, got %q", prefix)
	}
	if joined := strings.Join(importCmd.Args, " "); strings.Contains(joined, "/tmp/govard-") {
		t.Fatalf("the import must not stage a file on the remote: %s", joined)
	}
}
