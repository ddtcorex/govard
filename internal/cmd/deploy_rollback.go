package cmd

import (
	"context"
	"fmt"
	"io"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var deployRollbackCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapSSH) + "," + string(runtime.CapRsync),
	},
	Use:   "rollback [remote]",
	Short: "Put an earlier release back in front of the target",
	Long: `Roll back to a release that is still on the target. Nothing is rebuilt and
nothing is uploaded: the release directory is already there.

With a symlink layout the current symlink is re-pointed, which takes seconds.
With an in-place layout the publish tail is re-run from the existing release
directory — activate, flush caches, re-record — so the code that goes live is
the code that was already deployed, not a fresh build of it.

--with-db additionally restores the database dump that release was taken with.
That destroys data, so it needs --yes (or an interactive confirmation).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployRollback,
}

// DeployRollbackCommand exposes the rollback subcommand for tests.
func DeployRollbackCommand() *cobra.Command { return deployRollbackCmd }

func init() {
	deployRollbackCmd.Flags().String("remote", "", "Remote environment to roll back (alternative to the positional argument)")
	deployRollbackCmd.Flags().String("to", "", "Release number or revision to roll back to (default: the release before the live one)")
	deployRollbackCmd.Flags().Bool("with-db", false, "Also restore the database dump recorded by that release")
	deployRollbackCmd.Flags().Bool("yes", false, "Do not prompt for confirmation")
}

func runDeployRollback(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	host, err := deployHostFor(cmd.Context(), config, remote, options, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if err := confirmProtectedRemote(cmd, config, remote, options, "Roll back"); err != nil {
		return err
	}

	ctx := cmd.Context()
	to, _ := cmd.Flags().GetString("to")
	withDB, _ := cmd.Flags().GetBool("with-db")

	previous, err := deploy.LiveRelease(ctx, host)
	if err != nil {
		return err
	}
	target, err := deploy.SelectRollbackTarget(ctx, host, to)
	if err != nil {
		return err
	}
	// The dump a rollback restores belongs to the release that ran *after* the
	// target: that deploy dumped the database before its own migrations, which is
	// the state the target expects. The target's own dump is older than the code
	// being restored, which is what this command used to restore.
	var dumpRecord *deploy.Release
	if withDB {
		dumpRecord, err = deploy.RollbackBackup(ctx, host, target)
		if err != nil {
			return err
		}
		if err := confirmDatabaseRestore(cmd, remote, dumpRecord, options); err != nil {
			return err
		}
	}

	strategy, err := deploy.ResolvePublishStrategy(host, options)
	if err != nil {
		return err
	}
	// The rollback activates an existing release: the revision is the one that
	// release was built from, never the branch tip, and --force keeps the no-op
	// fast path from concluding there is nothing to do.
	options.Publish = strategy
	options.Revision = target.Revision
	options.Tag = ""
	options.Force = true

	hooks, err := hooksFromConfig(options.Hooks)
	if err != nil {
		return configOrUsageError(err)
	}
	recipe, options, err := deployRecipe(config, options)
	if err != nil {
		return configOrUsageError(err)
	}
	plan, err := deploy.BuildPlan(recipe, hooks)
	if err != nil {
		return configOrUsageError(err)
	}
	// A rollback publishes with the same strategy a deploy would, so the plan has
	// to be shaped the same way: in place that is what takes the maintenance
	// window out of the migration probe's hands, and on a symlink it is what
	// closes the window before the swap.
	plan = plan.ForPublishStrategy(strategy)
	vars := deployVars(host, options)
	out := cmd.OutOrStdout()

	// A rollback changes what the target serves, exactly like a deploy does, so
	// it takes the same lock and holds it for the whole operation: a deploy that
	// started in the middle of one would fight it for the symlink and the release
	// record. Refusing while somebody else holds the lock is the point.
	if !options.SkipLock {
		lockCtx := &deploy.StepContext{
			Host:    host,
			Runner:  host.Runner(),
			Vars:    deploy.NewVars(),
			Release: target,
			Opts:    options,
			Out:     out,
		}
		if err := deploy.CoreLock(ctx, lockCtx); err != nil {
			return err
		}
		defer func() {
			if err := deploy.CoreUnlock(context.WithoutCancel(ctx), lockCtx); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "  ! the deploy lock could not be released: %v\n", err)
			}
		}()
	}

	pterm.Info.Printf("rolling %s back to release %s (%s)\n", remote, target.Release, deploy.ShortRevision(target.Revision))

	if strategy == deploy.PublishSymlink {
		activate := firstStep(plan, deploy.TaskActivate)
		if activate.ID == "" {
			return fmt.Errorf("the recipe declares no %s task, so nothing can be activated", deploy.TaskActivate)
		}
		if err := deploy.RunStep(ctx, host, options, vars, activate, target, out); err != nil {
			return fmt.Errorf("re-point %s at release %s: %w", host.CurrentPath, target.Release, err)
		}
		// The swap is atomic on disk, but the application the target serves was
		// built for the release that was live a moment ago: its compiled
		// configuration, its caches and any Redis state belong to the code that
		// just went away. An in-place rollback flushes through its tail; a symlink
		// rollback runs nothing else, so the flush is explicit — below, after a
		// possible restore, because a cache rebuilt from a database that is about to
		// be replaced is the state the restore exists to undo.
	} else {
		// The tail is handed the fact that a restore follows: `deploy:verify` runs
		// inside it, and the framework's own checks query the database
		// (`setup:db:status`, `migrate:status`), so verifying before the restore
		// compares the release being restored against the schema it is about to
		// replace.
		if err := runInPlaceRollbackTail(ctx, host, options, vars, plan, target, out, withDB); err != nil {
			return err
		}
	}

	// The order is the whole point: restore, then flush, then verify. Each step's
	// input is the previous step's output — the caches are rebuilt from the
	// database, and the verification queries it — so any other order verifies or
	// warms state that is about to be replaced. On a symlink the flush is the only
	// one of the three the rollback runs itself; in place the tail already ran one,
	// and a restore is what makes a second one necessary.
	restore := func() error {
		if !withDB {
			return nil
		}
		return restoreReleaseDatabase(cmd, host, options, vars, plan, recipe, target, dumpRecord)
	}
	flush := func() error {
		if strategy != deploy.PublishSymlink && !withDB {
			return nil
		}
		step := firstStep(plan, deploy.TaskAppCacheFlush)
		if step.ID == "" {
			return nil
		}
		if err := deploy.RunStep(ctx, host, options, vars, step, target, out); err != nil {
			return fmt.Errorf("flush the caches of release %s after the rollback: %w", target.Release, err)
		}
		return nil
	}
	verify := func() error {
		if !options.Verify {
			return nil
		}
		step := firstStep(plan, deploy.TaskVerify)
		if step.ID == "" {
			return nil
		}
		return deploy.RunStep(ctx, host, options, vars, step, target, out)
	}
	if err := afterRestore(restore, flush, verify); err != nil {
		return err
	}

	pterm.Success.Printf("%s now serves release %s (%s)\n", remote, target.Release, deploy.ShortRevision(target.Revision))
	if previous != nil && previous.Release != target.Release {
		pterm.Info.Printf("it replaced release %s; `govard deploy releases %s` shows every release on the target\n", previous.Release, remote)
	}
	return nil
}

// afterRestore runs a rollback's post-activation work in the one order that is
// correct once a database is involved, skipping the steps that do not apply and
// stopping at the first failure. It is a named function rather than three
// consecutive statements because the order *is* the fix: a flush before a restore
// repopulates the caches from the data being replaced, and a verification before
// it compares the restored release against a schema it is about to replace.
func afterRestore(restore, flush, verify func() error) error {
	for _, step := range []func() error{restore, flush, verify} {
		if step == nil {
			continue
		}
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// restoreReleaseDatabase brackets the restore with the recipe's maintenance
// tasks. The dump is loaded with the site down, because a half-restored database
// served to traffic is worse than a short outage.
func restoreReleaseDatabase(cmd *cobra.Command, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, recipe deploy.Recipe, target, dump *deploy.Release) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if enable := firstStep(plan, deploy.TaskMaintenanceEnable); enable.ID != "" {
		if err := deploy.RunStep(ctx, host, options, vars, enable, target, out); err != nil {
			return fmt.Errorf("enable maintenance mode before restoring the database: %w", err)
		}
	}

	// The command runs in the release being restored (`{{release_path}}`, so a
	// Magento restore lands its dump inside that release's `var/backups`), but
	// the dump it loads is the one the release after it recorded. A copy carries
	// both facts without rewriting the target's record, which the deploy:record
	// step of an in-place tail would otherwise persist with someone else's dump.
	restore := *target
	restore.Database.Backup = dump.Database.Backup
	pterm.Info.Printf("restoring the dump recorded by release %s (%s) into the database\n", dump.Release, dump.Database.Backup)

	sc := &deploy.StepContext{
		Host:    host,
		Runner:  host.Runner(),
		Vars:    vars.Set("release", target.Release).SetPath("release_path", target.Path),
		Release: &restore,
		Opts:    options,
		Out:     out,
	}
	restoreErr := deploy.CoreDBRestore(recipe.Restore)(ctx, sc)

	if disable := firstStep(plan, deploy.TaskMaintenanceDisable); disable.ID != "" {
		if err := deploy.RunStep(ctx, host, options, vars, disable, target, out); err != nil && restoreErr == nil {
			return fmt.Errorf("disable maintenance mode after restoring the database: %w", err)
		}
	}
	return restoreErr
}

// confirmDatabaseRestore is the second gate on the only destructive path in this
// command. With no terminal there is nothing to confirm with, so a missing --yes
// is a usage error rather than an assumption.
func confirmDatabaseRestore(cmd *cobra.Command, remote string, dump *deploy.Release, options deploy.Options) error {
	if options.Yes {
		return nil
	}
	prompt := fmt.Sprintf("Restore the dump release %s recorded before its migrations over the live database on %s? This destroys current data.", dump.Release, remote)
	if !stdinIsTerminal() {
		return &cli.UsageError{Err: fmt.Errorf("%s; pass --yes to confirm", prompt)}
	}
	confirm, err := pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show(prompt)
	if err != nil {
		return err
	}
	if !confirm {
		return &cli.UsageError{Err: fmt.Errorf("database restore on %s cancelled", remote)}
	}
	return nil
}

// firstStep returns the named step of a plan, or the zero Step when the recipe
// does not implement it.
func firstStep(plan deploy.Plan, id string) deploy.Step {
	steps := plan.Only(id).Steps
	if len(steps) == 0 {
		return deploy.Step{}
	}
	return steps[0]
}

// runInPlaceRollbackTail rewrites an in-place docroot back to an earlier release.
//
// The docroot is rewritten while it serves, exactly as a deploy rewrites it, so
// the window opens first: the tail starts at deploy:activate, which is *after*
// maintenance:enable, so shaping the plan is not enough on its own. If the tail
// fails the window stays open — the same policy the deploy lock follows — and the
// error says so, because a site left in maintenance looks like an outage to
// whoever finds it.
//
// The lock belongs to the caller, and pruning releases is not part of returning
// to one: deploy:unlock would remove the lock this run took, and deploy:cleanup
// could prune the release it is rolling back from.
//
// restoreAfterTail says a database restore follows this tail. `deploy:verify` runs
// inside the tail and its framework checks query the database, so with a restore
// pending it is held back: verifying the release against a schema that is about to
// be replaced fails the rollback for the very state the restore is fixing.
func runInPlaceRollbackTail(ctx context.Context, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, target *deploy.Release, out io.Writer, restoreAfterTail bool) error {
	tail, ok := plan.From(deploy.TaskActivate)
	if !ok {
		return fmt.Errorf("the recipe declares no %s task, so an in-place rollback cannot run", deploy.TaskActivate)
	}
	tail = tail.Excluding(deploy.TaskUnlock, deploy.TaskCleanup)
	if restoreAfterTail {
		tail = tail.Excluding(deploy.TaskVerify)
	}

	if enable := firstStep(plan, deploy.TaskMaintenanceEnable); enable.ID != "" && !enable.Skipped {
		if err := deploy.RunStep(ctx, host, options, vars, enable, target, out); err != nil {
			return fmt.Errorf("enable maintenance mode before rewriting %s: %w", host.CurrentPath, err)
		}
	}
	// The tail runs the executor over the target's own record, and the executor
	// rewrites that record as it goes — storing `failed` when a step fails. That
	// record is what `deploy rollback --to <T>` reads to decide which releases are
	// usable, so a failed attempt would take a healthy release out of every future
	// rollback: the operator's way back, destroyed by the attempt to use it. The
	// record is restored on failure, and the rollback then simply did not happen.
	original := *target
	if _, err := deploy.NewExecutor(host, options, out).Run(ctx, tail, vars, target); err != nil {
		// Best effort, like every other record write on a failure path: the step's
		// error is the one that matters, and a record that cannot be written is
		// reported rather than allowed to hide it.
		if restoreErr := deploy.WriteRelease(context.WithoutCancel(ctx), host, &original); restoreErr != nil {
			fmt.Fprintf(out, "  ! the record of release %s could not be restored: %v\n", original.Release, restoreErr)
		}
		// The lock belongs to the caller, which releases it as this returns, and
		// `--resume` is the wrong command here: the release being rolled back to is
		// not the unfinished one. The command that finishes the job is the one that
		// started it.
		return fmt.Errorf("%w (the maintenance window is still open on %s: release %s and its record are as they were, and the deploy lock is released, so re-run `govard deploy --remote %s rollback --to %s` once the cause is fixed)",
			err, host.Name, original.Release, host.Name, original.Release)
	}
	return nil
}

// RunInPlaceRollbackTailForTest exposes runInPlaceRollbackTail to the tests/
// package: the ordering it owns — window, then the rewrite — and the verify step
// it holds back when a restore follows are the behaviour the tests pin, rather
// than the plan shape they hand it.
func RunInPlaceRollbackTailForTest(ctx context.Context, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, target *deploy.Release, out io.Writer, restoreAfterTail bool) error {
	return runInPlaceRollbackTail(ctx, host, options, vars, plan, target, out, restoreAfterTail)
}

// AfterRestoreForTest exposes afterRestore to the tests/ package, so the order it
// fixes is pinned by a test rather than by reading the caller.
func AfterRestoreForTest(restore, flush, verify func() error) error {
	return afterRestore(restore, flush, verify)
}
