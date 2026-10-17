//go:build !windows

package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"govard/internal/desktop"
)

// Hostile-umask mode assertions for database transfer secrets: syscall.Umask
// does not exist on Windows, so these live behind !windows. The production
// Chmod code stays unguarded (portable; read-only bit on Windows).

func TestDBTransfer_SecretFileModeIgnoresUmask(t *testing.T) {
	previous := syscall.Umask(0o700)
	t.Cleanup(func() { syscall.Umask(previous) })

	args, envFile, cleanup, err := desktop.BuildDBTransferDockerArgsForTest(
		"sample-project-db-1", "s3cret-test-pw", "MYSQL_PWD", "echo hi",
	)
	if err != nil {
		t.Fatalf("BuildDBTransferDockerArgs error = %v, want nil", err)
	}
	defer cleanup()
	for _, arg := range args {
		if strings.Contains(arg, "s3cret-test-pw") {
			t.Errorf("docker arg %q leaks the database password", arg)
		}
	}
	info, err := os.Stat(envFile)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("env file perm under restrictive umask = %o, want 600 (explicit chmod, not umask luck)", perm)
	}
}

func TestDBDump_DestFileModeIgnoresUmask(t *testing.T) {
	dir := t.TempDir()
	previous := syscall.Umask(0o700)
	t.Cleanup(func() { syscall.Umask(previous) })

	writer, err := desktop.CreateDBDumpDestForTest(filepath.Join(dir, "dump.sql.partial"))
	if err != nil {
		t.Fatalf("CreateDBDumpDest error = %v, want nil", err)
	}
	if _, err := io.WriteString(writer, "x"); err != nil {
		_ = writer.Close()
		t.Fatalf("write dest: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close dest: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "dump.sql.partial"))
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("dest file perm under restrictive umask = %o, want 600 (explicit chmod, not umask luck)", perm)
	}
}
