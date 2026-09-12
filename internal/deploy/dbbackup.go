package deploy

import (
	"context"
	"errors"
	"fmt"
	"path"
)

// ErrNoDatabaseBackup means a rollback asked to restore a dump the target
// release never recorded. Refusing is deliberate: restoring "something" onto a
// live database is worse than not restoring at all.
var ErrNoDatabaseBackup = errors.New("the release recorded no database backup")

// backupFileName is the dump's name inside the release's backup directory.
const backupFileName = "dump.sql"

// CoreDBBackup wraps a framework's dump command into the `db:backup` task.
//
// The split is deliberate: the engine owns where the dump goes and that it is
// recorded in `release.json`, because rollback reads that record; the framework
// owns how to dump its own database, because that is the only part that differs.
// The command template may reference {{backup_path}}.
//
// With `--db-backup` off nothing happens: the step is a no-op rather than a
// failure, exactly like verification with `--no-verify`.
func CoreDBBackup(command string) TaskFunc {
	return func(ctx context.Context, sc *StepContext) error {
		if !sc.Opts.DBBackup {
			return nil
		}
		if sc.Release == nil {
			return fmt.Errorf("db:backup: no release record")
		}

		directory := sc.Host.SharedBackupPath(sc.Release.Release)
		target := path.Join(directory, backupFileName)
		if _, err := sc.Runner.Run(ctx, "mkdir -p "+Shell(directory), RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("create the backup directory %s: %w", directory, err)
		}
		if err := runDBCommand(ctx, sc, command, target, "db:backup"); err != nil {
			return err
		}

		sc.Release.Database.Backup = target
		return nil
	}
}

// CoreDBRestore wraps a framework's restore command into a rollback step. It
// reads the path from the release record, never from a flag, so the dump that is
// restored is the one taken for the release being rolled back to.
func CoreDBRestore(command string) TaskFunc {
	return func(ctx context.Context, sc *StepContext) error {
		if sc.Release == nil {
			return fmt.Errorf("db:restore: no release record")
		}
		backup := sc.Release.Database.Backup
		if backup == "" {
			return fmt.Errorf("%w: %s", ErrNoDatabaseBackup, sc.Release.Release)
		}
		if _, err := sc.Runner.Run(ctx, "test -r "+Shell(backup), RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("the recorded backup %s is not readable on the target: %w", backup, err)
		}
		return runDBCommand(ctx, sc, command, backup, "db:restore")
	}
}

func runDBCommand(ctx context.Context, sc *StepContext, command, backupPath, label string) error {
	if command == "" {
		return fmt.Errorf("%s: this framework's recipe provides no dump command", label)
	}
	vars := sc.Vars.SetRaw("backup_path", backupPath)
	expanded, err := vars.Expand(command)
	if err != nil {
		return fmt.Errorf("expand %s: %w", label, err)
	}
	if _, err := sc.Runner.Run(ctx, expanded, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
