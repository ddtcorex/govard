package cmd

import (
	"io"
	"os/exec"
	"strings"
	"testing"

	"govard/internal/engine"
)

func testDBCredentials() dbCredentials {
	return dbCredentials{
		Engine:   "mariadb",
		Host:     "db.internal",
		Port:     3306,
		Username: "magento",
		Password: "s3cr3t-argv",
		Database: "magento",
	}
}

func assertNoSecretInArgv(t *testing.T, cmd *exec.Cmd, secret string) {
	t.Helper()
	for _, arg := range cmd.Args {
		if strings.Contains(arg, secret) {
			t.Fatalf("argument %q carries the password", arg)
		}
	}
}

func assertSecretInEnv(t *testing.T, cmd *exec.Cmd, secret string) {
	t.Helper()
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "MYSQL_PWD=") && strings.Contains(env, secret) {
			return
		}
	}
	t.Fatalf("no MYSQL_PWD in the process environment (%v)", cmd.Env)
}

func TestLocalDBCommandsKeepThePasswordOutOfArgv(t *testing.T) {
	credentials := testDBCredentials()

	cases := map[string]*exec.Cmd{
		"dump":   buildLocalDBDumpCommand("sample-project-db-1", credentials, false, false, "magento2"),
		"import": buildLocalDBImportCommand("sample-project-db-1", credentials),
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			assertNoSecretInArgv(t, cmd, credentials.Password)
			assertSecretInEnv(t, cmd, credentials.Password)
			joined := strings.Join(cmd.Args, " ")
			if !strings.Contains(joined, "-e MYSQL_PWD ") && !strings.HasSuffix(joined, "-e MYSQL_PWD") {
				if !strings.Contains(joined, "MYSQL_PWD") {
					t.Fatalf("args %v do not pass MYSQL_PWD by name", cmd.Args)
				}
			}
		})
	}
}

func TestRemoteDBCommandsReadTheScriptFromStdin(t *testing.T) {
	script := buildRemoteMySQLDumpCommandString(testDBCredentials(), false, false, "magento2", true)
	if !strings.Contains(script, "MYSQL_PWD") {
		t.Fatal("the dump script lost its credential hand-off")
	}

	cmd := sshScriptCommand("staging", engine.RemoteConfig{Host: "staging.example.invalid", User: "deploy"}, script)
	assertNoSecretInArgv(t, cmd, "s3cr3t-argv")
	if cmd.Stdin == nil {
		t.Fatal("the script does not travel on stdin")
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.HasSuffix(joined, "sh -s") {
		t.Fatalf("remote command %q does not read the script from stdin", joined)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "mysqldump") || strings.Contains(arg, "MYSQL_PWD") {
			t.Fatalf("the script leaked into argv: %q", arg)
		}
	}
}

// TestRemoteImportStagesItsScriptKeepsTheSecretOutOfArgv pins the import path,
// whose stdin carries the SQL: the script is staged on the remote and the ssh
// command that runs it carries only its path.
func TestRemoteImportStagesItsScriptKeepsTheSecretOutOfArgv(t *testing.T) {
	var staged []recordedStage
	original := runSSHStdin
	runSSHStdin = func(cmd *exec.Cmd) ([]byte, error) {
		script, _ := io.ReadAll(cmd.Stdin)
		staged = append(staged, recordedStage{args: cmd.Args, stdin: string(script)})
		return nil, nil
	}
	t.Cleanup(func() { runSSHStdin = original })

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "unknown",
		Remotes: map[string]engine.RemoteConfig{
			"dev": {
				Host:         "example.invalid",
				User:         "deploy",
				Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(true)},
			},
		},
	}
	importCmd, _, err := buildDBImportCommand(config, dbCommandOptions{Environment: "dev", File: "backup.sql", NoNoise: true})
	if err != nil {
		t.Fatalf("buildDBImportCommand: %v", err)
	}

	if len(staged) != 1 {
		t.Fatalf("staged %d scripts, want one", len(staged))
	}
	joinedStage := strings.Join(staged[0].args, " ")
	if strings.Contains(joinedStage, "MYSQL_PWD") || strings.Contains(joinedStage, "mysql ") {
		t.Fatalf("the staging command carries the script in argv: %s", joinedStage)
	}
	if !strings.HasPrefix(staged[0].stdin, "umask 077 && cat > ") {
		t.Fatalf("the staging command does not read the script from stdin: %q", staged[0].stdin)
	}

	joinedRun := strings.Join(importCmd.Args, " ")
	if !strings.Contains(joinedRun, "rm -f ") {
		t.Fatalf("the import command does not clean up the staged script: %s", joinedRun)
	}
	if strings.Contains(joinedRun, "MYSQL_PWD") || strings.Contains(joinedRun, "mysqldump") {
		t.Fatalf("the import command carries the script in argv: %s", joinedRun)
	}
}

type recordedStage struct {
	args  []string
	stdin string
}
