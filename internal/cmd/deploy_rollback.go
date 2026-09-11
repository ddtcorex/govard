package cmd

import (
	"fmt"

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
	if withDB {
		if target.Database.Backup == "" {
			return fmt.Errorf("release %s on %s recorded no database dump; re-run without --with-db", target.Release, remote)
		}
		if err := confirmDatabaseRestore(cmd, remote, target, options); err != nil {
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
	recipe, options := deployRecipe(config, options)
	plan, err := deploy.BuildPlan(recipe, hooks, remote)
	if err != nil {
		return configOrUsageError(err)
	}
	vars := deployVars(host, options)
	out := cmd.OutOrStdout()

	pterm.Info.Printf("rolling %s back to release %s (%s)\n", remote, target.Release, shortRevisionForOutput(target.Revision))

	if strategy == deploy.PublishSymlink {
		activate := firstStep(plan, deploy.TaskActivate)
		if activate.ID == "" {
			return fmt.Errorf("the recipe declares no %s task, so nothing can be activated", deploy.TaskActivate)
		}
		if err := deploy.RunStep(ctx, host, options, vars, activate, target, out); err != nil {
			return fmt.Errorf("re-point %s at release %s: %w", host.CurrentPath, target.Release, err)
		}
	} else {
		tail, ok := plan.From(deploy.TaskActivate)
		if !ok {
			return fmt.Errorf("the recipe declares no %s task, so an in-place rollback cannot run", deploy.TaskActivate)
		}
		if _, err := deploy.NewExecutor(host, options, out).Run(ctx, tail, vars, target); err != nil {
			return err
		}
	}

	if withDB {
		if err := restoreReleaseDatabase(cmd, host, options, vars, plan, recipe, target); err != nil {
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

	pterm.Success.Printf("%s now serves release %s (%s)\n", remote, target.Release, shortRevisionForOutput(target.Revision))
	if previous != nil && previous.Release != target.Release {
		pterm.Info.Printf("it replaced release %s; `govard deploy releases %s` shows every release on the target\n", previous.Release, remote)
	}
	return nil
}

// restoreReleaseDatabase brackets the restore with the recipe's maintenance
// tasks. The dump is loaded with the site down, because a half-restored database
// served to traffic is worse than a short outage.
func restoreReleaseDatabase(cmd *cobra.Command, host deploy.Host, options deploy.Options, vars deploy.Vars, plan deploy.Plan, recipe deploy.Recipe, target *deploy.Release) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if enable := firstStep(plan, deploy.TaskMaintenanceEnable); enable.ID != "" {
		if err := deploy.RunStep(ctx, host, options, vars, enable, target, out); err != nil {
			return fmt.Errorf("enable maintenance mode before restoring the database: %w", err)
		}
	}
	pterm.Info.Printf("restoring %s into the database\n", target.Database.Backup)

	sc := &deploy.StepContext{
		Host:    host,
		Runner:  host.Runner(),
		Vars:    vars.Set("release", target.Release).SetPath("release_path", target.Path),
		Release: target,
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
func confirmDatabaseRestore(cmd *cobra.Command, remote string, target *deploy.Release, options deploy.Options) error {
	if options.Yes {
		return nil
	}
	prompt := fmt.Sprintf("Restore the %s dump over the live database on %s? This destroys current data.", target.Release, remote)
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
