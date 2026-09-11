package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

// deployLockStaleAfter is how old a lock must be before `unlock` releases it
// without --force. A deploy that legitimately takes longer than this should
// raise the value rather than be interrupted.
const deployLockStaleAfter = 2 * time.Hour

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

	holder, heldFor := describeLockHolder(cmd, host)
	if !force && heldFor >= 0 && heldFor < deployLockStaleAfter {
		return fmt.Errorf("%s holds a lock started %s ago by %s; pass --force to release it", remote, heldFor.Round(time.Second), holder)
	}

	if err := deploy.CoreUnlock(ctx, deploy.StepContextForTest(host, options)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "released the deploy lock on %s (held by %s)\n", remote, holder)
	return nil
}

// describeLockHolder reads owner.json. "unknown" is a legitimate answer: the
// lock may predate the fields, and refusing to unlock because the metadata is
// unreadable would be worse than releasing it with --force.
func describeLockHolder(cmd *cobra.Command, host deploy.Host) (string, time.Duration) {
	result, err := host.Runner().Run(cmd.Context(), "cat "+deploy.Shell(host.LockOwnerPath()), deploy.RunOptions{Timeout: time.Minute})
	if err != nil {
		return "unknown", -1
	}
	var owner struct {
		Actor     string `json:"actor"`
		Revision  string `json:"revision"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &owner); err != nil {
		return "unknown", -1
	}

	who := strings.TrimSpace(owner.Actor)
	if who == "" {
		who = "unknown"
	}
	if revision := strings.TrimSpace(owner.Revision); revision != "" {
		who += " at " + shortRevisionForOutput(revision)
	}

	started, err := time.Parse(time.RFC3339, strings.TrimSpace(owner.StartedAt))
	if err != nil {
		return who, -1
	}
	return who, time.Since(started)
}
