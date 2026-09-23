//go:build unix

package tests

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"govard/internal/deploy"
)

// A dump holds customer data — orders, addresses, password hashes. The engine
// owns the directory it writes into, and the default umask on a shared host
// leaves both the directory and the file readable by every local user, so the
// modes are set explicitly rather than inherited.
func TestDBBackupDirectoryAndFileArePrivate(t *testing.T) {
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })

	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")

	sc := deploy.StepContextForTest(host, deploy.Options{DBBackup: true})
	sc.Release = release

	if err := deploy.CoreDBBackup(`printf dump > {{backup_path}}`)(context.Background(), sc); err != nil {
		t.Fatalf("db:backup: %v", err)
	}

	for _, row := range []struct {
		path string
		want os.FileMode
	}{
		{host.BackupRootPath(), 0o700},
		{filepath.Dir(release.Database.Backup), 0o700},
		{release.Database.Backup, 0o600},
	} {
		info, err := os.Stat(row.path)
		if err != nil {
			t.Fatalf("stat %s: %v", row.path, err)
		}
		if got := info.Mode().Perm(); got != row.want {
			t.Errorf("%s mode = %o, want %o", row.path, got, row.want)
		}
	}
}
