package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: "ssh,rsync",
	},
	Use:   "deploy [remote]",
	Short: "Deploy a revision to a remote environment",
	Long: `Deploy a git revision to a remote environment.

The target is a remote from .govard.yml. Everything else comes from the project
configuration: the branch, the repository, the deploy path and the publish
strategy, each of which a remote may override.

The pipeline is a fixed sequence of neutral tasks (see docs/reference/cli-commands.md).
A project customises it by anchoring hooks on a task id, on a stage alias
(stage:build) or on another hook, in .govard.yml.

--resume continues the newest unfinished release; when the remote holds no
unfinished release, the command starts a new one and says so.

This command needs only ssh and rsync: it runs on a host without Docker, which is
what lets the same command work in CI and on a development machine.

Exit codes: 0 success, 1 execution failure, 2 usage, 3 missing capability,
4 configuration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeploy,
}

func init() {
	bindDeployFlags(deployCmd)

	deployUnlockCmd.Flags().Bool("force", false, "Release a lock that is not stale")

	// plan and check read the same source and build-mode flags as deploy: the
	// plan has to express the mode it is planning, and the preflight has to
	// know an artifact is coming before it can compare it with the target.
	bindDeploySourceFlags(deployPlanCmd)
	deployPlanCmd.Flags().String("build", deploy.BuildAuto, "Where the build runs: auto, server or artifact")
	deployPlanCmd.Flags().String("artifact-dir", "", "Artifact directory built by govard deploy build")
	deployPlanCmd.Flags().Bool("json", false, "Emit machine-readable output")

	bindDeploySourceFlags(deployCheckCmd)
	deployCheckCmd.Flags().String("build", deploy.BuildAuto, "Where the build runs: auto, server or artifact")
	deployCheckCmd.Flags().String("artifact-dir", "", "Artifact directory built by govard deploy build")

	deployReleasesCmd.Flags().Bool("json", false, "Emit machine-readable output")
	deployStatusCmd.Flags().Bool("json", false, "Emit machine-readable output")

	// The read commands resolve the remote through deployRemoteName, which
	// already accepts the flag: bind it so --remote works uniformly across
	// the whole deploy group instead of positional-only on these three.
	for _, readCmd := range []*cobra.Command{deployReleasesCmd, deployStatusCmd, deployUnlockCmd} {
		readCmd.Flags().String("remote", "", "Remote environment (alternative to the positional argument)")
	}

	deployCmd.AddCommand(deployPlanCmd)
	deployCmd.AddCommand(deployCheckCmd)
	deployCmd.AddCommand(deployReleasesCmd)
	deployCmd.AddCommand(deployStatusCmd)
	deployCmd.AddCommand(deployRollbackCmd)
	deployCmd.AddCommand(deployUnlockCmd)

	rootCmd.AddCommand(deployCmd)
}

// DeployCommand exposes the deploy command for tests.
func DeployCommand() *cobra.Command { return deployCmd }

func runDeploy(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	noteOut := cmd.OutOrStdout()
	if options.JSON {
		noteOut = cmd.ErrOrStderr()
	}
	host, err := deployHostFor(cmd.Context(), config, remote, options, noteOut)
	if err != nil {
		return err
	}
	if err := confirmProtectedRemote(cmd, config, remote, options, "Deploy "+options.Revision+" to"); err != nil {
		return err
	}

	strategy, err := deploy.ResolvePublishStrategy(host, options)
	if err != nil {
		return err
	}
	options.Publish = strategy

	if options.Revision == "" && options.Tag == "" {
		revision, err := resolveLocalRevision(cmd.Context())
		if err != nil {
			return err
		}
		options.Revision = revision
	}

	hooks, err := hooksFromConfig(options.Hooks)
	if err != nil {
		return configOrUsageError(err)
	}
	recipe, options, err := deployRecipe(config, options)
	if err != nil {
		return configOrUsageError(err)
	}
	plan, err := deployPlanFor(recipe, hooks, options)
	if err != nil {
		return configOrUsageError(err)
	}
	// A --from that names nothing would silently run the whole pipeline, which
	// is the opposite of what the operator asked for.
	if options.From != "" && plan.IndexOf(options.From) < 0 {
		return &cli.UsageError{Err: fmt.Errorf("--from %q does not name a task or hook in this deploy plan (see `govard deploy plan %s`)", options.From, remote)}
	}
	// A --from past deploy:release continues a release, and the only release a
	// run may continue is the one an earlier attempt recorded. Past that task the
	// run has no release number at all: `{{release_path}}` would be the directory
	// holding every release, and an activation would publish all of them. Refused
	// here, where the message can name the fix.
	if err := validateFromNeedsResume(plan, options.From, options.Resume); err != nil {
		return &cli.UsageError{Err: err}
	}

	// The local lifecycle hooks keep working: pre_deploy wraps the pipeline,
	// post_deploy follows it, exactly as before this command did anything real.
	if err := engine.RunHooks(config, engine.HookPreDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("pre-deploy hooks failed: %w", err)
	}

	// `--json` reserves stdout for one JSON document, so everything a human
	// would read — the stage timeline, the discovery note, the warning about a
	// missing verify URL — goes to stderr instead. A CI job pipes stdout into a
	// parser and keeps stderr in the job log.
	timeline := cmd.OutOrStdout()
	if options.JSON {
		timeline = cmd.ErrOrStderr()
	}

	release := deploy.NewRelease("", options.Revision, options.Branch)
	if options.Resume {
		resumed, err := prepareResume(cmd.Context(), host, release, options, timeline)
		if err != nil {
			return err
		}
		if resumed == nil {
			if !options.JSON {
				pterm.Info.Printf("%s has no unfinished release; starting a new one\n", remote)
			}
		} else {
			// The record's revision is the release's identity: every command
			// the recipe expands and the content version must describe the
			// revision being resumed, not the local HEAD resolved a moment ago.
			options.Revision = release.Revision
			options.Tag = ""
		}
	}

	outcome, runErr := deploy.NewExecutor(host, options, timeline).Run(cmd.Context(), plan, deployVars(host, options), release)

	if hookErr := engine.RunHooks(config, engine.HookPostDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); hookErr != nil && runErr == nil {
		return fmt.Errorf("post-deploy hooks failed: %w", hookErr)
	}
	if runErr != nil {
		// A failure is reported in the same document shape as a success: CI
		// reads one contract, and the human explanation goes to stderr with the
		// returned error.
		if options.JSON {
			writeDeployJSON(cmd, remote, options, release, outcome, runErr)
		}
		return fmt.Errorf("%w\n%s", runErr, deploy.RecoveryHint(remote, outcome))
	}

	printDeploySummary(cmd, remote, host, options, release, outcome)
	return nil
}

// lockIsHeld answers whether the deploy lock is on the target, and refuses to
// guess. `test -d` says "no" with exit 1, while a transport error says nothing at
// all about the lock — and reading the second as the first is how a resume
// released the lock of a deploy that was still running and put two runs on one
// release directory, migrations included. A lock that cannot be checked is
// reported as its own failure, and the caller treats it as held.
func lockIsHeld(ctx context.Context, host deploy.Host, release string) (bool, error) {
	_, err := host.Runner().Run(ctx, "test -d "+deploy.Shell(host.LockPath()), deploy.RunOptions{Timeout: time.Minute})
	if err == nil {
		return true, nil
	}
	var commandErr *deploy.CommandError
	if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
		return false, nil
	}
	return false, fmt.Errorf("release %s cannot be resumed: the deploy lock could not be checked on %s, so it is treated as held rather than released: %w", release, host.Name, err)
}

// prepareResume points the run at the newest unfinished release and clears the
// lock its failed run left behind. Recovery has to be explicit, so the operator
// asks for it with --resume rather than a retry silently taking over.
//
// The release the run continues is the stored record itself, copied whole onto
// the caller's value. Rebuilding it from a few fields looks equivalent and is
// not: `CoreVerify` reads Publish.Strategy and `deploy rollback --with-db` reads
// Database.Backup, so a partial record verifies nothing and loses the dump path.
func prepareResume(ctx context.Context, host deploy.Host, release *deploy.Release, options deploy.Options, out io.Writer) (*deploy.Release, error) {
	incomplete, err := deploy.ResumeTarget(ctx, host)
	if err != nil {
		return nil, err
	}
	if incomplete == nil {
		return nil, nil
	}

	// A record that is still `running` belongs to a deploy that may be running
	// right now. Resuming it would release that run's lock and put two deploys on
	// the same release directory, migrations included — exactly what the lock
	// exists to prevent. The run is only ours to continue when its lock is gone
	// or old enough that no live process can be holding it, which is the same
	// judgement `deploy unlock` makes.
	if incomplete.Status == deploy.StatusRunning {
		lockHeld, err := lockIsHeld(ctx, host, incomplete.Release)
		if err != nil {
			return nil, err
		}
		if lockHeld {
			holder, heldFor := deploy.DescribeLockOwner(ctx, host)
			staleAfter := options.LockStaleAfter
			if staleAfter <= 0 {
				staleAfter = deploy.DefaultLockStaleAfter
			}
			if heldFor < 0 || heldFor < staleAfter {
				return nil, fmt.Errorf("release %s is still running: %s holds its lock (held %s); wait for that run, or release the lock with `govard deploy unlock %s` once it is gone — for a lock younger than `deploy.lock_stale_after` that needs `--force`, because age is the only evidence govard has that the holder is dead",
					incomplete.Release, holder, heldFor.Round(time.Second), host.Name)
			}
		}
	}

	// Never resume backwards. A failed release *older* than the live one is
	// history: a CI retry that always passes --resume would otherwise activate an
	// older revision over a newer release that is serving traffic.
	//
	// Equality is not backwards, it is the normal case. A run that failed at or
	// after `publish:activate` — a hook, the cache flush, the verification — has
	// already made its own release the one the target serves: `current` points at
	// it, and in place the docroot's HEAD is its revision. That failed record is
	// therefore both the newest unfinished release and the live one, and refusing
	// it would leave the maintenance window it opened with no resume to close it.
	// Only a strictly older record is refused.
	if live, err := deploy.LiveRelease(ctx, host); err == nil && live != nil {
		if target, convErr := strconv.Atoi(strings.TrimSpace(incomplete.Release)); convErr == nil {
			if liveNumber, convErr := strconv.Atoi(strings.TrimSpace(live.Release)); convErr == nil && target < liveNumber {
				return nil, fmt.Errorf("release %s is older than the live release %s, so resuming it would activate an older revision over the one serving traffic; deploy normally, or return to it with `govard deploy rollback`",
					incomplete.Release, live.Release)
			}
		}
	}

	*release = *incomplete

	// The stale lock belongs to the run being resumed; releasing it is part of
	// resuming, and the operator asked for that explicitly. The context carries
	// the run's own options and output rather than a test helper's zero value.
	unlock := &deploy.StepContext{
		Host:    host,
		Runner:  host.Runner(),
		Vars:    deploy.NewVars(),
		Release: release,
		Opts:    options,
		Out:     out,
	}
	if err := deploy.CoreUnlock(ctx, unlock); err != nil {
		return nil, err
	}
	pterm.Info.Printf("resuming release %s (previously failed)\n", release.Release)
	return release, nil
}

// PrepareResumeForTest exposes prepareResume to the tests/ package. The
// invariant it pins — that a resume continues the stored record whole — is the
// whole reason the function exists, so it is tested through this rather than
// restated in the caller.
func PrepareResumeForTest(ctx context.Context, host deploy.Host, release *deploy.Release) error {
	_, err := prepareResume(ctx, host, release, deploy.Options{}, io.Discard)
	return err
}

// PrepareResumeWithOptionsForTest exposes prepareResume with the run's options,
// which carry the staleness window the live-lock guard reads.
func PrepareResumeWithOptionsForTest(ctx context.Context, host deploy.Host, release *deploy.Release, options deploy.Options) error {
	_, err := prepareResume(ctx, host, release, options, io.Discard)
	return err
}

// confirmProtectedRemote refuses to deploy to a protected environment without an
// explicit confirmation. With no terminal there is nothing to confirm with, so a
// missing --yes is a usage error rather than an assumption.
func confirmProtectedRemote(cmd *cobra.Command, config engine.Config, remote string, options deploy.Options, action string) error {
	var remoteCfg engine.RemoteConfig
	if strings.ToLower(strings.TrimSpace(remote)) == deploy.SandboxRemoteName {
		resolved, _, err := resolveSandboxRemote(cmd.Context(), config.ProjectName)
		if err != nil {
			return err
		}
		remoteCfg = resolved
	} else {
		found, ok := config.Remotes[remote]
		if !ok {
			return nil
		}
		remoteCfg = found
	}
	blocked, reason := engine.RemoteWriteBlocked(remote, remoteCfg)
	if !blocked || options.Yes {
		return nil
	}
	if !stdinIsTerminal() {
		return &cli.UsageError{Err: fmt.Errorf("%s is protected (%s); pass --yes to run non-interactively", remote, reason)}
	}
	confirm, err := pterm.DefaultInteractiveConfirm.
		WithDefaultValue(false).
		Show(fmt.Sprintf("%s protected remote %q?", action, remote))
	if err != nil {
		return err
	}
	if !confirm {
		return &cli.UsageError{Err: fmt.Errorf("operation on %s cancelled", remote)}
	}
	return nil
}

// ConfirmProtectedRemoteForTest exposes confirmProtectedRemote to the tests/ package.
func ConfirmProtectedRemoteForTest(cmd *cobra.Command, config engine.Config, remote string, options deploy.Options, action string) error {
	return confirmProtectedRemote(cmd, config, remote, options, action)
}

// resolveLocalRevision defaults the target revision to the local HEAD, which is
// what makes "deploy what I have" the zero-flag behaviour on a development
// machine. CI always passes --revision.
func resolveLocalRevision(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("resolve the local revision (pass --revision in CI): %w", err)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", fmt.Errorf("resolve the local revision: git returned an empty value")
	}
	return revision, nil
}

func printDeploySummary(cmd *cobra.Command, remote string, host deploy.Host, options deploy.Options, release *deploy.Release, outcome deploy.Outcome) {
	if options.JSON {
		writeDeployJSON(cmd, remote, options, release, outcome, nil)
		return
	}

	if outcome.AlreadyDeployed {
		pterm.Info.Printf("%s already runs %s; nothing to do (use --force to deploy again)\n", remote, deploy.ShortRevision(release.Revision))
		return
	}
	pterm.Success.Printf("Deployed %s to %s as release %s in %s\n",
		deploy.ShortRevision(release.Revision), remote, release.Release, outcome.Total.Round(1e6))
	if options.Publish == deploy.PublishInPlace {
		pterm.Info.Printf("Publish strategy: in_place (%s)\n", host.CurrentPath)
	} else {
		pterm.Info.Printf("Publish strategy: symlink (%s -> %s)\n", host.CurrentPath, release.Path)
	}
	if !options.Verify {
		pterm.Warning.Println("Verification was skipped (--no-verify)")
	}
}

// validateFromNeedsResume refuses a `--from` that starts *after* deploy:release.
//
// `--from` starts the pipeline part-way through, which only makes sense for a
// release that already exists: everything that would have created it —
// deploy:release, and with it the release number every later command expands
// into `{{release_path}}` — was skipped. Without --resume there is no record to
// read the number from, and the engine then works from an empty one. The engine
// refuses that too, but the CLI can name the fix.
//
// The line is the release task, not "any --from": a `--from` at or before
// deploy:release still *runs* it, so the run creates its own number and needs no
// resume. Refusing those too was broader than the rule the docs state, and it
// blocked a legitimate partial run.
func validateFromNeedsResume(plan deploy.Plan, from string, resume bool) error {
	from = strings.TrimSpace(from)
	if from == "" || resume {
		return nil
	}
	releaseIndex := plan.IndexOf(deploy.TaskRelease)
	if releaseIndex >= 0 {
		if fromIndex := plan.IndexOf(from); fromIndex >= 0 && fromIndex <= releaseIndex {
			return nil
		}
	}
	return fmt.Errorf("--from %q starts after deploy:release, so the run has no release number to expand `{{release_path}}` into: add --resume, or drop --from", from)
}

// ValidateFromNeedsResumeForTest exposes validateFromNeedsResume to the tests/
// package.
func ValidateFromNeedsResumeForTest(plan deploy.Plan, from string, resume bool) error {
	return validateFromNeedsResume(plan, from, resume)
}
