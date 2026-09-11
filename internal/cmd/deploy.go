package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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
	host, err := deploy.HostForConfig(config, remote, options)
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
		return &cli.UsageError{Err: err}
	}
	recipe, options := deployRecipe(config, options)
	plan, err := deployPlanFor(recipe, hooks, remote, options)
	if err != nil {
		return &cli.UsageError{Err: err}
	}
	// A --from that names nothing would silently run the whole pipeline, which
	// is the opposite of what the operator asked for.
	if options.From != "" && plan.IndexOf(options.From) < 0 {
		return &cli.UsageError{Err: fmt.Errorf("--from %q does not name a task or hook in this deploy plan (see `govard deploy plan %s`)", options.From, remote)}
	}

	// The local lifecycle hooks keep working: pre_deploy wraps the pipeline,
	// post_deploy follows it, exactly as before this command did anything real.
	if err := engine.RunHooks(config, engine.HookPreDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("pre-deploy hooks failed: %w", err)
	}

	release := deploy.NewRelease("", options.Revision, options.Branch)
	if options.Resume {
		resumed, err := prepareResume(cmd.Context(), host, release)
		if err != nil {
			return err
		}
		if resumed == nil {
			pterm.Info.Printf("%s has no unfinished release; starting a new one\n", remote)
		} else {
			// The record's revision is the release's identity: every command
			// the recipe expands and the content version must describe the
			// revision being resumed, not the local HEAD resolved a moment ago.
			options.Revision = release.Revision
			options.Tag = ""
		}
	}

	outcome, runErr := deploy.NewExecutor(host, options, cmd.OutOrStdout()).Run(cmd.Context(), plan, deployVars(host, options), release)

	if hookErr := engine.RunHooks(config, engine.HookPostDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); hookErr != nil && runErr == nil {
		return fmt.Errorf("post-deploy hooks failed: %w", hookErr)
	}
	if runErr != nil {
		return fmt.Errorf("%w\nthe release directory and its record were kept on %s; inspect them with `govard deploy check %s` before retrying", runErr, remote, remote)
	}

	printDeploySummary(cmd, remote, host, options, release, outcome)
	return nil
}

// prepareResume points the run at the newest unfinished release and clears the
// lock its failed run left behind. Recovery has to be explicit, so the operator
// asks for it with --resume rather than a retry silently taking over.
//
// The release the run continues is the stored record itself, copied whole onto
// the caller's value. Rebuilding it from a few fields looks equivalent and is
// not: `CoreVerify` reads Publish.Strategy and `deploy rollback --with-db` reads
// Database.Backup, so a partial record verifies nothing and loses the dump path.
func prepareResume(ctx context.Context, host deploy.Host, release *deploy.Release) (*deploy.Release, error) {
	incomplete, err := deploy.ResumeTarget(ctx, host)
	if err != nil {
		return nil, err
	}
	if incomplete == nil {
		return nil, nil
	}

	*release = *incomplete

	// The stale lock belongs to the run being resumed; releasing it is part of
	// resuming, and the operator asked for that explicitly.
	if err := deploy.CoreUnlock(ctx, deploy.StepContextForTest(host, deploy.Options{})); err != nil {
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
	_, err := prepareResume(ctx, host, release)
	return err
}

// confirmProtectedRemote refuses to deploy to a protected environment without an
// explicit confirmation. With no terminal there is nothing to confirm with, so a
// missing --yes is a usage error rather than an assumption.
func confirmProtectedRemote(cmd *cobra.Command, config engine.Config, remote string, options deploy.Options, action string) error {
	remoteCfg, ok := config.Remotes[remote]
	if !ok {
		return nil
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
		writeDeployJSON(cmd, remote, options, release, outcome)
		return
	}

	if outcome.AlreadyDeployed {
		pterm.Info.Printf("%s already runs %s; nothing to do (use --force to deploy again)\n", remote, shortRevisionForOutput(release.Revision))
		return
	}
	pterm.Success.Printf("Deployed %s to %s as release %s in %s\n",
		shortRevisionForOutput(release.Revision), remote, release.Release, outcome.Total.Round(1e6))
	if options.Publish == deploy.PublishInPlace {
		pterm.Info.Printf("Publish strategy: in_place (%s)\n", host.CurrentPath)
	} else {
		pterm.Info.Printf("Publish strategy: symlink (%s -> %s)\n", host.CurrentPath, release.Path)
	}
	if !options.Verify {
		pterm.Warning.Println("Verification was skipped (--no-verify)")
	}
}

func shortRevisionForOutput(revision string) string {
	if len(revision) > 8 {
		return revision[:8]
	}
	return revision
}
