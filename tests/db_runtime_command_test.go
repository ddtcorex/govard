package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestDBCommandRuntimeInfoAndQueryUseDockerShim(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "docker.log")
	installDBCommandRuntimeDockerShim(t, shimDir)
	t.Setenv("DB_COMMAND_RUNTIME_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	infoOut := &bytes.Buffer{}
	root.SetOut(infoOut)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"db", "info"})
	if err := root.Execute(); err != nil {
		t.Fatalf("db info failed: %v", err)
	}

	output := infoOut.String()
	if !strings.Contains(output, "Database Connection Info") {
		t.Fatalf("db info output missing header:\n%s", output)
	}
	if !strings.Contains(output, "Environment:  local") {
		t.Fatalf("db info output missing local environment:\n%s", output)
	}

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"db", "query", "SELECT 1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("db query failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, "docker|inspect -f {{.State.Running}} sample-project-db-1") {
		t.Fatalf("missing running inspect in docker log:\n%s", logs)
	}
	if !strings.Contains(logs, "docker|exec -i -e MYSQL_PWD sample-project-db-1 sh -lc") {
		t.Fatalf("missing query exec invocation in docker log:\n%s", logs)
	}
}

func TestDBCommandRuntimeDumpWritesFileWithDockerShim(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "docker.log")
	installDBCommandRuntimeDockerShim(t, shimDir)
	t.Setenv("DB_COMMAND_RUNTIME_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dumpPath := filepath.Join(tempDir, "dump.sql")
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"db", "dump", "--file", dumpPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("db dump failed: %v", err)
	}

	dumpContent, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump file: %v", err)
	}
	if !strings.Contains(string(dumpContent), "CREATE TABLE demo") {
		t.Fatalf("unexpected dump file content:\n%s", string(dumpContent))
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, "docker|exec -i -e MYSQL_PWD sample-project-db-1 sh -lc if command -v mariadb-dump") {
		t.Fatalf("missing dump exec invocation in docker log:\n%s", logs)
	}
}

func installDBCommandRuntimeDockerShim(t *testing.T, shimDir string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
log="${DB_COMMAND_RUNTIME_LOG:-}"
if [ -n "$log" ]; then
  printf 'docker|%s\n' "$*" >> "$log"
fi
if [ "${1:-}" = "inspect" ] && [ "${2:-}" = "-f" ]; then
  case "${3:-}" in
    "{{.State.Running}}")
      echo "true"
      exit 0
      ;;
    "{{range .Config.Env}}{{println .}}{{end}}")
      cat <<'EOF'
MYSQL_USER=dbuser
MYSQL_PASSWORD=dbpass
MYSQL_DATABASE=dbname
EOF
      exit 0
      ;;
  esac
fi
if [ "${1:-}" = "exec" ]; then
  case "$*" in
    *mysqldump*)
      printf 'CREATE TABLE demo (id INT);\n'
      exit 0
      ;;
    *)
      exit 0
      ;;
  esac
fi
exit 0
`
	path := filepath.Join(shimDir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
}
