package cmd

import (
	"fmt"
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
clearing the lock an explicit act: this command refuses a recent lock unless
--force is given, and names the run holding it.`,
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
	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	host, err := deployHostFor(cmd.Context(), config, remote, options, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")

	ctx := cmd.Context()
	if _, statErr := host.Runner().Run(ctx, "test -d "+deploy.Shell(host.LockPath()), deploy.RunOptions{Timeout: time.Minute}); statErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s holds no deploy lock\n", remote)
		return nil
	}

	holder, heldFor := deploy.DescribeLockOwner(ctx, host)
	if !force && heldFor >= 0 && heldFor < options.LockStaleAfter {
		return fmt.Errorf("%s holds a lock started %s ago by %s; pass --force to release it", remote, heldFor.Round(time.Second), holder)
	}

	if err := deploy.CoreUnlock(ctx, deploy.StepContextForTest(host, options)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "released the deploy lock on %s (held by %s)\n", remote, holder)
	return nil
}
