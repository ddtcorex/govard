package deploy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// ErrNoDatabaseBackup means a rollback asked to restore a dump the target
// release never recorded. Refusing is deliberate: restoring "something" onto a
// live database is worse than not restoring at all.
var ErrNoDatabaseBackup = errors.New("the release recorded no database backup")

// backupFileName is the dump's name inside the release's backup directory.
const backupFileName = "dump.sql"

// ValidateDBBackup refuses `--db-backup` for a recipe that has no dump command.
//
// The executor skips a task carrying neither a command nor a core implementation
// without consulting anything, so without this check an operator who asked for a
// backup immediately before a destructive `db:migrate` would get a successful
// deploy, a `db:backup … skipped` line, no dump and no error — the failure mode
// the flag exists to prevent. Refusing is a configuration error (exit 4) and it
// happens before the run starts, so nothing has been published when it is
// reported.
//
// The message names the flag, the recipe and the anchor a project can use
// instead: a `deploy.hooks` entry on `db:backup` runs next to the skipped task,
// which is the documented way to add a dump the framework does not provide.
func ValidateDBBackup(recipe Recipe, opts Options) error {
	if !opts.DBBackup {
		return nil
	}
	if !recipe.Task(TaskDBBackup).IsEmpty() {
		return nil
	}
	return fmt.Errorf(
		"%w: %q: --db-backup needs a dump command this recipe does not provide; "+
			"drop the flag, or take the dump yourself with a deploy.hooks entry anchored on db:backup",
		ErrInvalidConfiguration, recipe.ID,
	)
}

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
		// The dump holds customer data, and the process umask of a deploy account
		// is whatever the host set: the directory and the file are made private
		// explicitly instead of inheriting 0755/0644 on a shared machine.
		mkdir := "umask 077 && mkdir -p " + Shell(directory) + " && chmod 700 " + Shell(sc.Host.BackupRootPath()) + " " + Shell(directory)
		if _, err := sc.Runner.Run(ctx, mkdir, RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("create the backup directory %s: %w", directory, err)
		}
		if err := runDBCommand(ctx, sc, command, target, "db:backup"); err != nil {
			return err
		}
		// A recipe that moves its dump and one that copies it both keep whatever
		// mode the writer gave the file, so the mode is set here, not assumed.
		if _, err := sc.Runner.Run(ctx, "chmod 600 "+Shell(target), RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("make the backup %s private: %w", target, err)
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
	// A path, not raw text: a deploy path with a space (or any shell
	// metacharacter) would otherwise reach the command unquoted and the dump
	// would be written somewhere else, or nowhere.
	vars := sc.Vars.SetPath("backup_path", backupPath)
	expanded, err := vars.Expand(command)
	if err != nil {
		return fmt.Errorf("expand %s: %w", label, err)
	}
	if _, err := sc.Runner.Run(ctx, expanded, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

// ErrNoRollbackBackup means the release that ran after the rollback target
// recorded no dump. The database cannot be returned to the state the target
// expects, and guessing — the target's own dump, or a newer release's — would
// restore a schema that does not match the code being put back.
var ErrNoRollbackBackup = errors.New("the release after the rollback target recorded no database backup")

// RollbackBackup returns the release whose dump a `rollback --with-db` restores.
//
// A dump is taken before its own release migrates, so the dump of the release
// that ran *after* the target is the state the target expects: rolling back to
// T undoes the migrations of T+1…live, and T+1's dump is the database exactly as
// it was before the first of them. The target's own dump is the state before
// *its* migrations — an older schema than the code being restored — which is why
// reading the target's record, as this command used to, restored the wrong data.
//
// Refusing beats guessing: when that release recorded no dump (it deployed
// without --db-backup) the honest answer is that this rollback cannot restore a
// database, not a dump from some other release.
func RollbackBackup(ctx context.Context, host Host, target *Release) (*Release, error) {
	if target == nil {
		return nil, fmt.Errorf("rollback: no target release")
	}
	targetNumber, err := strconv.Atoi(strings.TrimSpace(target.Release))
	if err != nil {
		return nil, fmt.Errorf("rollback: the target release %q is not a release number", target.Release)
	}

	entries, err := ListReleases(ctx, host)
	if err != nil {
		return nil, err
	}
	next := 0
	for _, entry := range entries {
		if entry.Foreign || entry.Number <= targetNumber {
			continue
		}
		if next == 0 || entry.Number < next {
			next = entry.Number
		}
	}
	if next == 0 {
		return nil, fmt.Errorf("%w: release %s is the newest release on %s, so no later release has a dump to restore",
			ErrNoRollbackBackup, target.Release, host.Name)
	}

	record, err := ReadRelease(ctx, host, strconv.Itoa(next))
	if err != nil {
		return nil, fmt.Errorf("read release %d: %w", next, err)
	}
	if strings.TrimSpace(record.Database.Backup) == "" {
		return nil, fmt.Errorf("%w: release %d ran after release %s and recorded none; deploy it again with --db-backup, or roll back without --with-db",
			ErrNoRollbackBackup, next, target.Release)
	}
	return record, nil
}
