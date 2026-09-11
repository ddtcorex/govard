package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestDBBackupIsANoOpUnlessRequested(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")

	// The command would fail loudly if it ran at all: --db-backup defaults off,
	// and an optional step that nobody asked for must not touch the database.
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})
	sc.Release = release

	if err := deploy.CoreDBBackup("exit 9")(context.Background(), sc); err != nil {
		t.Fatalf("db:backup without --db-backup returned %v, want a no-op", err)
	}
	if release.Database.Backup != "" {
		t.Fatalf("release recorded a backup %q that was never taken", release.Database.Backup)
	}
}

func TestDBBackupWritesIntoTheReleasesBackupDirectoryAndRecordsIt(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("3", "abcdef", "main")
	release.Path = host.ReleasePath("3")

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", DBBackup: true})
	sc.Release = release

	command := `printf 'dump' > {{backup_path}}`
	if err := deploy.CoreDBBackup(command)(context.Background(), sc); err != nil {
		t.Fatalf("db:backup: %v", err)
	}

	want := filepath.Join(host.DeployPath, "shared", "backups", "deploy", "3", "dump.sql")
	if release.Database.Backup != want {
		t.Fatalf("recorded backup = %q, want %q", release.Database.Backup, want)
	}
	content, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("the dump was not written: %v", err)
	}
	if string(content) != "dump" {
		t.Fatalf("dump content = %q, want %q", content, "dump")
	}
}

func TestDBRestoreRefusesAReleaseWithoutARecordedBackup(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("2", "abcdef", "main")
	release.Path = host.ReleasePath("2")

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})
	sc.Release = release

	err := deploy.CoreDBRestore("true")(context.Background(), sc)
	if !errors.Is(err, deploy.ErrNoDatabaseBackup) {
		t.Fatalf("err = %v, want ErrNoDatabaseBackup", err)
	}
}

func TestDBRestoreReadsThePathFromTheReleaseRecord(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	backup := filepath.Join(host.DeployPath, "shared", "backups", "deploy", "2", "dump.sql")
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		t.Fatalf("prepare backup: %v", err)
	}
	if err := os.WriteFile(backup, []byte("dump"), 0o644); err != nil {
		t.Fatalf("prepare backup: %v", err)
	}

	release := deploy.NewReleaseForTest("2", "abcdef", "main")
	release.Path = host.ReleasePath("2")
	release.Database.Backup = backup

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})
	sc.Release = release

	var seen []string
	sc.Runner = captureRunner{base: deploy.LocalRunner{}, seen: &seen}

	if err := deploy.CoreDBRestore("test -r {{backup_path}}")(context.Background(), sc); err != nil {
		t.Fatalf("db:restore: %v", err)
	}
	if !strings.Contains(strings.Join(seen, "\n"), backup) {
		t.Fatalf("the restore command did not receive the recorded path: %q", seen)
	}
}

// captureRunner records the commands it is asked to run, so a test can assert
// which path a step actually received.
type captureRunner struct {
	base deploy.Runner
	seen *[]string
}

func (r captureRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	*r.seen = append(*r.seen, command)
	return r.base.Run(ctx, command, opts)
}
