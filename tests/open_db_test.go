package tests

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// openDBSecretPasswords cover a quote and a dollar sign (shell-hostile) and a
// plain token, so the raw, URL-escaped and shell-quoted forms all differ.
var openDBSecretPasswords = []string{`p'w$x`, "SECRETPW"}

func openDBLeakForms(password string) []string {
	return []string{
		password,
		url.QueryEscape(password),
		url.PathEscape(password),
		engine.ShellQuote(password),
	}
}

func installOpenDBPasswordDockerShim(t *testing.T, shimDir string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "inspect" ] && [ "${2:-}" = "-f" ]; then
  case "${3:-}" in
    "{{.State.Running}}")
      echo "true"
      exit 0
      ;;
    "{{range .Config.Env}}{{println .}}{{end}}")
      printf 'MYSQL_USER=dbuser\nMYSQL_PASSWORD=%s\nMYSQL_DATABASE=dbname\n' "$OPEN_DB_SHIM_PASSWORD"
      exit 0
      ;;
  esac
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(shimDir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
}

// installOpenDBTunnelSSHShim installs an ssh stand-in that, when asked for a
// -L forward, listens on the local port for a short while and then exits. The
// listener is a python3 one-liner, so the test skips on a host without python3.
// The lifetime only has to outlast the 100ms readiness poll and the opener spawn.
func installOpenDBTunnelSSHShim(t *testing.T, shimDir string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required by the tunnel ssh shim and is not on PATH")
	}
	script := `#!/bin/sh
set -eu
port=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-L" ]; then
    port="${2%%:*}"
    break
  fi
  shift
done
[ -n "$port" ] || exit 1
exec python3 - "$port" <<'PY'
import socket, sys, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(sys.argv[1])))
s.listen(4)
time.sleep(0.6)
PY
`
	if err := os.WriteFile(filepath.Join(shimDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write ssh shim: %v", err)
	}
}

func executeCapturingPterm(t *testing.T, root *cobra.Command) (string, error) {
	t.Helper()
	var captured bytes.Buffer
	pterm.SetDefaultOutput(&captured)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	err := root.Execute()
	return captured.String(), err
}

func assertOpenDBOutputHidesPassword(t *testing.T, output string, password string) {
	t.Helper()
	for _, form := range openDBLeakForms(password) {
		if strings.Contains(output, form) {
			t.Fatalf("output leaks the password as %q:\n%s", form, output)
		}
	}
	if !strings.Contains(output, "Opening DB URL mysql://dbuser:***@127.0.0.1:") {
		t.Fatalf("expected the redacted URL line in output:\n%s", output)
	}
}

func assertOpenDBOpenerGotPassword(t *testing.T, openerLog string, password string) {
	t.Helper()
	want := cmd.BuildOpenDBConnectionURLForTest("dbuser", password, "dbname", 0)
	want = want[:strings.Index(want, "@127.0.0.1:")+len("@127.0.0.1:")]
	if !strings.Contains(openerLog, want) {
		t.Fatalf("opener must still receive the real URL %q, got:\n%s", want, openerLog)
	}
}

func TestOpenDBNeverPrintsThePassword(t *testing.T) {
	openerBinary, ok := openRuntimeOpenerBinary()
	if !ok || runtime.GOOS == "windows" {
		t.Skipf("open db runtime shims are not supported on %s", runtime.GOOS)
	}

	for _, password := range openDBSecretPasswords {
		t.Run("local client "+password, func(t *testing.T) {
			resetOpenFlagsForRuntimeTest(t)
			tempDir := t.TempDir()
			chdirForTest(t, tempDir)
			writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: laravel\n")

			shimDir := t.TempDir()
			openLog := filepath.Join(shimDir, "open.log")
			installOpenRuntimeShim(t, shimDir, openerBinary)
			installOpenDBPasswordDockerShim(t, shimDir)
			t.Setenv("OPEN_RUNTIME_LOG", openLog)
			t.Setenv("OPEN_DB_SHIM_PASSWORD", password)
			t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			root := cmd.RootCommandForTest()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"open", "db", "--client"})
			output, runErr := executeCapturingPterm(t, root)
			if runErr != nil {
				t.Fatalf("open db --client failed: %v\n%s", runErr, output)
			}

			assertOpenDBOutputHidesPassword(t, output, password)
			assertOpenDBOpenerGotPassword(t, readRuntimeLog(t, openLog), password)
		})

		t.Run("remote tunnel "+password, func(t *testing.T) {
			resetOpenFlagsForRuntimeTest(t)
			tempDir := t.TempDir()
			chdirForTest(t, tempDir)
			writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: custom\n"+
				"remotes:\n  stg:\n    host: staging.example.com\n    user: deploy\n    path: /srv/www/app\n"+
				"    db_user: dbuser\n    db_name: dbname\n    db_pass: "+"'"+strings.ReplaceAll(password, "'", "''")+"'\n")

			shimDir := t.TempDir()
			openLog := filepath.Join(shimDir, "open.log")
			installOpenRuntimeShim(t, shimDir, openerBinary)
			installOpenDBTunnelSSHShim(t, shimDir)
			t.Setenv("OPEN_RUNTIME_LOG", openLog)
			t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			root := cmd.RootCommandForTest()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"open", "db", "--environment", "stg"})
			output, runErr := executeCapturingPterm(t, root)
			if runErr != nil {
				t.Fatalf("open db remote failed: %v\n%s", runErr, output)
			}

			assertOpenDBOutputHidesPassword(t, output, password)
			assertOpenDBOpenerGotPassword(t, readRuntimeLog(t, openLog), password)
		})
	}
}

func TestRedactedOpenDBConnectionURLShape(t *testing.T) {
	if got := cmd.RedactedOpenDBConnectionURLForTest("dbuser", "SECRETPW", "shop", 13306); got != "mysql://dbuser:***@127.0.0.1:13306/shop" {
		t.Fatalf("unexpected redacted URL with password: %q", got)
	}
	if got := cmd.RedactedOpenDBConnectionURLForTest("dbuser", "", "shop", 13306); got != "mysql://dbuser@127.0.0.1:13306/shop" {
		t.Fatalf("unexpected redacted URL without password: %q", got)
	}
}
