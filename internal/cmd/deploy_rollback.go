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
		// rollback runs nothing else, so the flush has to be explicit here.
		if flush := firstStep(plan, deploy.TaskAppCacheFlush); flush.ID != "" {
			if err := deploy.RunStep(ctx, host, options, vars, flush, target, out); err != nil {
				return fmt.Errorf("flush the caches of release %s after re-pointing %s: %w", target.Release, host.CurrentPath, err)
			}
		}
	} else {
		if err := runInPlaceRollbackTail(ctx, host, options, vars, plan, target, out); err != nil {
			return err
		}
	}

	if withDB {
		if err := restoreReleaseDatabase(cmd, host, options, vars, plan, recipe, target, dumpRecord); err != nil {
			return err
		}
	}

	if options.Verify {
		if verify := firstStep(plan, deploy.TaskVerify); verify.ID != "" {
			if err := deploy.RunStep(ctx, host, options, vars, verify, target, out); err != nil {
				return err
			}
		}
	}

	pterm.Success.Printf("%s now serves release %s (%s)\n", remote, target.Release, deploy.ShortRevision(target.Revision))
	if previous != nil && previous.Release != target.Release {
		pterm.Info.Printf("it replaced release %s; `govard deploy releases %s` shows every release on the target\n", previous.Release, remote)
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
func runInPlaceRollbackTail(ctx context.Context, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, target *deploy.Release, out io.Writer) error {
	tail, ok := plan.From(deploy.TaskActivate)
	if !ok {
		return fmt.Errorf("the recipe declares no %s task, so an in-place rollback cannot run", deploy.TaskActivate)
	}
	tail = tail.Excluding(deploy.TaskUnlock, deploy.TaskCleanup)

	if enable := firstStep(plan, deploy.TaskMaintenanceEnable); enable.ID != "" && !enable.Skipped {
		if err := deploy.RunStep(ctx, host, options, vars, enable, target, out); err != nil {
			return fmt.Errorf("enable maintenance mode before rewriting %s: %w", host.CurrentPath, err)
		}
	}
	if _, err := deploy.NewExecutor(host, options, out).Run(ctx, tail, vars, target); err != nil {
		return fmt.Errorf("%w (the maintenance window is still open on %s: `govard deploy --resume` finishes the run, or `govard deploy unlock` releases the lock after closing it)", err, host.Name)
	}
	return nil
}

// RunInPlaceRollbackTailForTest exposes runInPlaceRollbackTail to the tests/
// package: the ordering it owns — window, then the rewrite — is the behaviour the
// test pins, rather than the plan shape it is handed.
func RunInPlaceRollbackTailForTest(ctx context.Context, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, target *deploy.Release, out io.Writer) error {
	return runInPlaceRollbackTail(ctx, host, options, vars, plan, target, out)
}
