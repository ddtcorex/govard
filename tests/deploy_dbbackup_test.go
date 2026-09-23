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
	sc := deploy.StepContextForTest(host, deploy.Options{})
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

	sc := deploy.StepContextForTest(host, deploy.Options{DBBackup: true})
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

	sc := deploy.StepContextForTest(host, deploy.Options{})
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

	sc := deploy.StepContextForTest(host, deploy.Options{})
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

// A dump is taken before its own release migrates, so the dump a rollback needs
// is the one belonging to the release that ran *after* the target: rolling back
// to 5 undoes release 6's migrations, and release 6's dump is the database as it
// was before them. Release 5's own dump is the state before *its* migrations —
// an older schema than the code being restored.
func TestRollbackRestoresTheDumpOfTheReleaseAfterTheTarget(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	for _, seed := range []struct{ number, dump string }{
		{"5", "/shared/backups/deploy/5/dump.sql"},
		{"6", "/shared/backups/deploy/6/dump.sql"},
	} {
		record := deploy.NewReleaseForTest(seed.number, "rev-"+seed.number, "main")
		record.Status = deploy.StatusOK
		record.Database.Backup = seed.dump
		if err := os.MkdirAll(host.ReleasePath(seed.number)+"/.dep", 0o755); err != nil {
			t.Fatalf("mkdir release %s: %v", seed.number, err)
		}
		if err := deploy.WriteRelease(context.Background(), host, record); err != nil {
			t.Fatalf("seed release %s: %v", seed.number, err)
		}
	}

	target, err := deploy.ReadRelease(context.Background(), host, "5")
	if err != nil {
		t.Fatalf("read the target: %v", err)
	}
	dump, err := deploy.RollbackBackup(context.Background(), host, target)
	if err != nil {
		t.Fatalf("rollback backup: %v", err)
	}
	if dump.Release != "6" {
		t.Fatalf("the dump must come from release 6, the release that ran after the target, got %s", dump.Release)
	}
	if !strings.HasSuffix(dump.Database.Backup, "/6/dump.sql") {
		t.Fatalf("wrong dump path: %s", dump.Database.Backup)
	}
}

// Guessing is worse than refusing: when the release that ran after the target
// recorded no dump there is no state to return to, and restoring an older
// release's dump would put the wrong schema under the restored code.
func TestRollbackRefusesWhenTheReleaseAfterTheTargetHasNoDump(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	five := deploy.NewReleaseForTest("5", "rev-5", "main")
	five.Status = deploy.StatusOK
	five.Database.Backup = "/shared/backups/deploy/5/dump.sql"
	six := deploy.NewReleaseForTest("6", "rev-6", "main")
	six.Status = deploy.StatusOK
	for _, record := range []*deploy.Release{five, six} {
		if err := os.MkdirAll(host.ReleasePath(record.Release)+"/.dep", 0o755); err != nil {
			t.Fatalf("mkdir release %s: %v", record.Release, err)
		}
		if err := deploy.WriteRelease(context.Background(), host, record); err != nil {
			t.Fatalf("seed release %s: %v", record.Release, err)
		}
	}

	target, err := deploy.ReadRelease(context.Background(), host, "5")
	if err != nil {
		t.Fatalf("read the target: %v", err)
	}
	_, err = deploy.RollbackBackup(context.Background(), host, target)
	if !errors.Is(err, deploy.ErrNoRollbackBackup) {
		t.Fatalf("want ErrNoRollbackBackup, got %v", err)
	}
	if !strings.Contains(err.Error(), "6") {
		t.Fatalf("the refusal must name release 6: %v", err)
	}
}
