package tests

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/runtime"
)

func TestResolveDBImportReaderForTestFromFile(t *testing.T) {
	tempDir := t.TempDir()
	dumpPath := filepath.Join(tempDir, "dump.sql")
	wantBody := "SELECT 1;\n"
	if err := os.WriteFile(dumpPath, []byte(wantBody), 0o644); err != nil {
		t.Fatalf("write dump file: %v", err)
	}

	reader, closer, _, err := cmd.ResolveDBImportReaderForTest(cmd.DBCommandOptions{File: dumpPath})
	if err != nil {
		t.Fatalf("ResolveDBImportReaderForTest() error = %v", err)
	}
	if closer == nil {
		t.Fatal("expected closer for file-backed reader")
	}
	t.Cleanup(func() {
		_ = closer.Close()
	})

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read import reader: %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("reader body = %q, want %q", string(body), wantBody)
	}
}

func TestResolveDBImportReaderForTestRejectsMissingInputOnTerminal(t *testing.T) {
	defer cmd.SetStdinIsTerminalForTest(func() bool { return true })()

	_, _, _, err := cmd.ResolveDBImportReaderForTest(cmd.DBCommandOptions{})
	if err == nil {
		t.Fatal("expected missing input error")
	}
	if !strings.Contains(err.Error(), "no import input provided") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveDBImportReaderForTestUsesStdinWhenPiped(t *testing.T) {
	defer cmd.SetStdinIsTerminalForTest(func() bool { return false })()

	reader, closer, _, err := cmd.ResolveDBImportReaderForTest(cmd.DBCommandOptions{})
	if err != nil {
		t.Fatalf("ResolveDBImportReaderForTest() error = %v", err)
	}
	if closer != nil {
		t.Fatal("expected nil closer for stdin reader")
	}
	if reader != os.Stdin {
		t.Fatal("expected stdin reader when terminal detection is false")
	}
}

func TestBuildDBDumpCommandForTestLocalUsesDockerInspectAndCredentials(t *testing.T) {
	shimDir := installDockerInspectShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	args, err := cmd.BuildDBDumpCommandForTest(
		engine.Config{ProjectName: "sample-project"},
		cmd.DBCommandOptions{Environment: "local"},
	)
	if err != nil {
		t.Fatalf("BuildDBDumpCommandForTest() error = %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "docker exec -i -e MYSQL_PWD=devpass sample-project-db-1 sh -lc") {
		t.Fatalf("unexpected local dump command (missing sh -lc): %s", joined)
	}
	if !strings.Contains(joined, "DUMP_BIN=mariadb-dump") {
		t.Fatalf("expected DUMP_BIN detection in command: %s", joined)
	}
	if !strings.Contains(joined, "--routines --triggers") {
		t.Fatalf("expected default dump flags in command: %s", joined)
	}
	if !strings.Contains(joined, "'devdb'") {
		t.Fatalf("expected resolved database name in command: %s", joined)
	}
}

func TestBuildDBImportCommandForTestLocalUsesDockerExecWithForceMode(t *testing.T) {
	shimDir := installDockerInspectShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	args, err := cmd.BuildDBImportCommandForTest(
		engine.Config{ProjectName: "sample-project"},
		cmd.DBCommandOptions{Environment: "local"},
	)
	if err != nil {
		t.Fatalf("BuildDBImportCommandForTest() error = %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "docker exec -i -e MYSQL_PWD=devpass sample-project-db-1 sh -lc") {
		t.Fatalf("unexpected local import command: %s", joined)
	}
	if !strings.Contains(joined, "-u 'devuser' 'devdb' -f") {
		t.Fatalf("expected mysql force import flags in command: %s", joined)
	}
}

func TestBuildDBDumpCommandForTestRemoteBuildsSSHCommand(t *testing.T) {
	config := engine.Config{
		Framework: "unknown",
		Remotes: map[string]engine.RemoteConfig{
			"dev": {
				Host: "example.com",
				User: "deploy",
				Capabilities: &engine.RemoteCapabilities{
					DB: engine.BoolPtr(true),
				},
			},
		},
	}

	args, err := cmd.BuildDBDumpCommandForTest(config, cmd.DBCommandOptions{Environment: "dev"})
	if err != nil {
		t.Fatalf("BuildDBDumpCommandForTest() error = %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "ssh ") {
		t.Fatalf("expected ssh command, got: %s", joined)
	}
	if !strings.Contains(joined, "deploy@example.com") {
		t.Fatalf("expected remote target in ssh command, got: %s", joined)
	}
	if !strings.Contains(joined, "mysqldump") {
		t.Fatalf("expected mysqldump command, got: %s", joined)
	}
}

func TestBuildDBImportCommandForTestRemoteBuildsSSHCommand(t *testing.T) {
	config := engine.Config{
		Framework: "unknown",
		Remotes: map[string]engine.RemoteConfig{
			"dev": {
				Host: "example.com",
				User: "deploy",
				Capabilities: &engine.RemoteCapabilities{
					DB: engine.BoolPtr(true),
				},
			},
		},
	}

	args, err := cmd.BuildDBImportCommandForTest(config, cmd.DBCommandOptions{Environment: "dev"})
	if err != nil {
		t.Fatalf("BuildDBImportCommandForTest() error = %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "ssh ") {
		t.Fatalf("expected ssh command, got: %s", joined)
	}
	if !strings.Contains(joined, "deploy@example.com") {
		t.Fatalf("expected remote target in ssh command, got: %s", joined)
	}
	if !strings.Contains(joined, "mysql") {
		t.Fatalf("expected mysql import command, got: %s", joined)
	}
}

func TestResolveDBRemoteForTestValidation(t *testing.T) {
	t.Run("UnknownRemote", func(t *testing.T) {
		_, err := cmd.ResolveDBRemoteForTest(engine.Config{Remotes: map[string]engine.RemoteConfig{}}, "dev", false)
		if err == nil {
			t.Fatal("expected unknown remote error")
		}
		if !strings.Contains(err.Error(), "unknown remote: dev") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("CapabilityMissing", func(t *testing.T) {
		config := engine.Config{
			Remotes: map[string]engine.RemoteConfig{
				"dev": {
					Capabilities: &engine.RemoteCapabilities{
						DB: engine.BoolPtr(false),
					},
				},
			},
		}
		_, err := cmd.ResolveDBRemoteForTest(config, "dev", false)
		if err == nil {
			t.Fatal("expected capability error")
		}
		if !strings.Contains(err.Error(), "does not allow db operations") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// §6.1: the gate protects writes, not reads. "prod" normalizes to the
	// production environment, so RemoteWriteBlocked is true without an explicit flag.
	t.Run("ProdAllowedForReadFlow", func(t *testing.T) {
		config := engine.Config{
			Remotes: map[string]engine.RemoteConfig{
				"prod": {
					Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(true)},
				},
			},
		}
		_, err := cmd.ResolveDBRemoteForTest(config, "prod", false)
		if err != nil {
			t.Fatalf("a read from a protected remote must be allowed, got: %v", err)
		}
	})

	t.Run("ProdBlockedForWriteFlow", func(t *testing.T) {
		config := engine.Config{
			Remotes: map[string]engine.RemoteConfig{
				"prod": {
					Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(true)},
				},
			},
		}
		_, err := cmd.ResolveDBRemoteForTest(config, "prod", true)
		if err == nil {
			t.Fatal("expected write-protection error")
		}
		if !strings.Contains(err.Error(), "write-protected") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestClassifyCommandErrorTreatsWriteProtectionAsValidation(t *testing.T) {
	err := fmt.Errorf("remote environment 'prod' is write-protected: production environment protection (auto)")
	if got := cmd.ClassifyCommandErrorForTest(err); got != "validation" {
		t.Fatalf("category = %q, want validation", got)
	}
}

// A remote whose name normalizes to production is auto-protected, so the gate
// refuses a write with no `protected: true` anywhere in the config — the same
// configuration shape an operator would have for `prod`.
func writeProtectedRemoteProject(t *testing.T, dir string) {
	t.Helper()
	writeRuntimeConfig(t, dir, `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  prod:
    host: prod.example.com
    user: deploy
    path: /srv/www/app
    capabilities:
      db: true
`)
}

// installLoggingShim puts an executable script named name at the front of PATH
// and points $SHIM_LOG at logPath, so a test can prove which external binaries
// the command under test really started, and in which order.
func installLoggingShim(t *testing.T, name, script, logPath string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write %s shim: %v", name, err)
	}
	t.Setenv("SHIM_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// dockerResetShim answers the local-database probes honestly (so the reset path
// is really reachable, not blocked by a container error) and records every call
// so "the local database was never touched" is an observation, not an assertion
// about code that was never run.
const dockerResetShim = `#!/bin/sh
printf 'docker %s\n' "$*" >> "$SHIM_LOG"
if [ "${1:-}" = "inspect" ] && [ "${2:-}" = "-f" ]; then
  case "${3:-}" in
    "{{.State.Running}}")
      echo "true"
      exit 0
      ;;
    "{{range .Config.Env}}{{println .}}{{end}}")
      printf 'MYSQL_USER=devuser\nMYSQL_PASSWORD=devpass\nMYSQL_DATABASE=devdb\n'
      exit 0
      ;;
  esac
fi
exit 0
`

// sshRefusalShim records every ssh argv and refuses to dial, so a test can prove
// what reached (or never reached) the wire.
const sshRefusalShim = `#!/bin/sh
printf 'ssh %s\n' "$*" >> "$SHIM_LOG"
echo "shim: refusing to dial" >&2
exit 1
`

// prepareDBCommandForTest clears the process-wide db flag state a previous
// Execute left behind and satisfies the declared container-runtime requirement:
// `db` is not in alwaysRunnableNames, so the root pre-run gate probes the host
// and a docker-less machine would answer exit 3 CAPABILITY_MISSING instead of
// the refusal under test.
func prepareDBCommandForTest(t *testing.T) {
	t.Helper()
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	t.Cleanup(restore)
	cmd.ResetDBFlagsForTest()
	t.Cleanup(cmd.ResetDBFlagsForTest)
}

func runDBCommandForTest(t *testing.T, args ...string) error {
	t.Helper()
	prepareDBCommandForTest(t)
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"db"}, args...))
	return root.Execute()
}

func writeImportDumpFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "backup.sql")
	if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatalf("write dump file: %v", err)
	}
	return path
}

// Review I1: `db import -e prod --drop` used to drop and recreate the LOCAL
// database and only then meet the write-protection gate inside
// buildDBImportCommand. The refusal has to come first, so `--drop`'s "yes" is
// never a path past the gate and no local data is destroyed on the way out.
func TestDBImportRefusesAProtectedRemoteBeforeTheDropConfirmation(t *testing.T) {
	prepareDBCommandForTest(t)

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeProtectedRemoteProject(t, tempDir)
	dumpPath := writeImportDumpFile(t, tempDir)

	dockerLog := filepath.Join(t.TempDir(), "docker.log")
	installLoggingShim(t, "docker", dockerResetShim, dockerLog)

	// A terminal is what makes the confirmation reachable at all, so the test
	// would catch the block being run before the gate. The prompt itself reads
	// the raw terminal, so the wait is bounded instead of hanging the suite.
	t.Cleanup(cmd.SetStdinIsTerminalForTest(func() bool { return true }))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"db", "import", "--environment", "prod", "--file", dumpPath, "--drop"})

	errCh := make(chan error, 1)
	go func() {
		errCh <- root.Execute()
	}()

	var err error
	select {
	case err = <-errCh:
	case <-time.After(15 * time.Second):
		t.Fatal("db import --drop reached the drop confirmation prompt; the write-protection refusal must arrive before it")
	}

	assertProtectedRemoteRefusal(t, err)
	if log := readRuntimeLog(t, dockerLog); log != "" {
		t.Fatalf("the local database was touched before the refusal:\n%s", log)
	}
}

// Same refusal, proved through the path where `-y` skips the confirmation: the
// only thing that can reset the local database there is the gate having already
// answered, so an untouched shim log means the DROP DATABASE never ran.
func TestDBImportRefusesAProtectedRemoteWithoutTouchingTheLocalDatabase(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeProtectedRemoteProject(t, tempDir)
	dumpPath := writeImportDumpFile(t, tempDir)

	dockerLog := filepath.Join(t.TempDir(), "docker.log")
	installLoggingShim(t, "docker", dockerResetShim, dockerLog)

	err := runDBCommandForTest(t, "import", "--environment", "prod", "--file", dumpPath, "--drop", "-y")

	assertProtectedRemoteRefusal(t, err)
	if log := readRuntimeLog(t, dockerLog); log != "" {
		t.Fatalf("the local database was touched before the refusal:\n%s", log)
	}
}

// Human ruling R1: `db query` interpolates the operator's SQL verbatim into
// `mysql -e '<SQL>'` ON THE REMOTE, and `db connect` is the same with the
// terminal attached. Both are writes against the remote whatever the SQL says,
// so both are refused. The assertions below drive the real commands, because a
// test that only calls ResolveDBRemoteForTest(config, name, true) would still
// pass with either call site passing false.
func TestDBQueryIsRefusedAgainstAProtectedRemote(t *testing.T) {
	const marker = "govard-write-protection-marker"
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeProtectedRemoteProject(t, tempDir)

	sshLog := filepath.Join(t.TempDir(), "ssh.log")
	installLoggingShim(t, "ssh", sshRefusalShim, sshLog)
	// The auth probe may fail; nothing may ever prompt on the way to the gate.
	t.Cleanup(cmd.SetStdinIsTerminalForTest(func() bool { return false }))

	err := runDBCommandForTest(t, "query", "--environment", "prod", "SELECT 1 /* "+marker+" */")

	if log := readRuntimeLog(t, sshLog); strings.Contains(log, marker) {
		t.Fatalf("the query reached the remote despite the refusal:\n%s", log)
	}
	assertProtectedRemoteRefusal(t, err)
}

func TestDBConnectIsRefusedAgainstAProtectedRemote(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeProtectedRemoteProject(t, tempDir)

	sshLog := filepath.Join(t.TempDir(), "ssh.log")
	installLoggingShim(t, "ssh", sshRefusalShim, sshLog)
	t.Cleanup(cmd.SetStdinIsTerminalForTest(func() bool { return false }))

	err := runDBCommandForTest(t, "connect", "--environment", "prod")

	if log := readRuntimeLog(t, sshLog); strings.Contains(log, "mysql") {
		t.Fatalf("a mysql client was started on the remote despite the refusal:\n%s", log)
	}
	assertProtectedRemoteRefusal(t, err)
}

// assertProtectedRemoteRefusal pins the refusal message and the exit code, both
// of which the gate already owned before this work: a plain execution error.
func assertProtectedRemoteRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a write-protection refusal against the protected remote")
	}
	if !strings.Contains(err.Error(), "is write-protected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cli.Code(err); got != cli.CodeError {
		t.Fatalf("exit code = %d, want %d", got, cli.CodeError)
	}
}

func installDockerInspectShim(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "inspect" ] && [ "${2:-}" = "-f" ]; then
  case "${3:-}" in
    "{{.State.Running}}")
      echo "true"
      exit 0
      ;;
    "{{range .Config.Env}}{{println .}}{{end}}")
      cat <<'EOF'
MYSQL_USER=devuser
MYSQL_PASSWORD=devpass
MYSQL_DATABASE=devdb
EOF
      exit 0
      ;;
  esac
fi
echo "unexpected docker args: $*" >&2
exit 1
`
	path := filepath.Join(dir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
	return dir
}
