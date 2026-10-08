package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

var deployUnlockCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapSSH),
	},
	Use:   "unlock [remote]",
	Short: "Release the deploy lock on a remote",
	Long: `Release the deploy lock a failed deploy left behind.

A deploy keeps its lock when it fails after publish, because the target may be
mid-change and a second deploy must not start against it silently. That makes
clearing the lock an explicit act: this command refuses a lock newer than
lock_stale_after (default 2h) unless --force is given, and names the run
holding it. It does not end maintenance mode: when the interrupted release left
it on, the output names the resume command that runs the disable step.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployUnlock,
}

// DeployUnlockCommand exposes the unlock subcommand for tests.
func DeployUnlockCommand() *cobra.Command { return deployUnlockCmd }

func runDeployUnlock(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployReadOptions(cmd, remote)
	if err != nil {
		return err
	}
	host, err := deployHostFor(cmd.Context(), config, remote, options, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	return unlockDeploy(cmd.Context(), host, remote, options, force, cmd.OutOrStdout())
}

// unlockDeploy releases the lock and then says what it did not touch.
func unlockDeploy(ctx context.Context, host deploy.Host, remote string, options deploy.Options, force bool, out io.Writer) error {
	if _, statErr := host.Runner().Run(ctx, "test -d "+deploy.Shell(host.LockPath()), deploy.RunOptions{Timeout: time.Minute}); statErr != nil {
		fmt.Fprintf(out, "%s holds no deploy lock\n", remote)
		printMaintenanceHint(ctx, host, remote, out)
		return nil
	}

	holder, heldFor := deploy.DescribeLockOwner(ctx, host)
	if !force && heldFor >= 0 && heldFor < options.LockStaleAfter {
		return fmt.Errorf("%s holds a lock started %s ago by %s; pass --force to release it", remote, heldFor.Round(time.Second), holder)
	}

	if err := deploy.CoreUnlock(ctx, &deploy.StepContext{
		Host:    host,
		Runner:  host.Runner(),
		Vars:    deploy.NewVars(),
		Release: &deploy.Release{},
		Opts:    options,
		Out:     out,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "released the deploy lock on %s (held by %s)\n", remote, holder)
	printMaintenanceHint(ctx, host, remote, out)
	return nil
}

// printMaintenanceHint warns when the newest unfinished release switched
// maintenance mode on and never off. Releasing the lock does not end the window,
// so the site keeps answering 503 until the recipe's own disable step runs, and
// that step is the recipe's: govard does not guess a framework command here.
func printMaintenanceHint(ctx context.Context, host deploy.Host, remote string, out io.Writer) {
	incomplete, err := deploy.IncompleteRelease(ctx, host)
	if err != nil || incomplete == nil || !incomplete.MaintenanceMayBeOn() {
		return
	}
	fmt.Fprintf(out, "note: release %s enabled maintenance mode and never disabled it, so the site may still answer 503; the lock does not control that. End it with `govard deploy --remote %s --resume --from %s`, which re-runs the recipe's disable step and everything after it\n",
		incomplete.Release, remote, deploy.TaskMaintenanceDisable)
}

// UnlockForTest exposes unlockDeploy to the tests/ package.
func UnlockForTest(ctx context.Context, host deploy.Host, remote string, options deploy.Options, force bool, out io.Writer) error {
	return unlockDeploy(ctx, host, remote, options, force, out)
}
