package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// deployBuildCmd is the CI half of the artifact mode: it runs where the
// project's toolchain lives and produces the directory `govard deploy
// --artifact-dir` consumes.
var deployBuildCmd = &cobra.Command{
	Annotations: map[string]string{
		// A build is local work: no ssh, no rsync, and no container runtime
		// unless the operator asks for one. `--runner container` is the single
		// flag that demands a container, and it asks at flag time through
		// requireDocker — the shape `audit run --checks` already uses, because a
		// static annotation cannot say "only when this flag is passed".
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "build [remote]",
	Short: "Build an artifact for --build=artifact without connecting to the target",
	Long: `Build an artifact locally: materialise the revision into the output directory,
run the recipe's build tasks there, and write a manifest describing the result.

This is the first of the two documented CI jobs. It runs on a runner that has the
project's toolchain (PHP, Composer, Node — whatever the recipe's build tasks
need). The second job, ` + "`govard deploy <remote> --artifact-dir <dir>`" + `, needs only
govard, ssh and rsync:

  govard deploy build production --output artifacts --revision $CI_COMMIT_SHA
  govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes

The build tasks run on the host shell, which is why the command itself declares no
capability. A project whose build cannot fetch its own dependencies from the host
can pass ` + "`--runner container`" + ` to run the same tasks through the project's own app
container instead. That flag needs a running project container, and an output
directory inside the project root — everything else on this page behaves
identically. A stopped or missing container exits 3 before the output directory
is touched. The container carries only its own toolchain: when the recipe's
frontend step will run, the build first checks the container for node and npm
and refuses (exit 3) if either is missing, so build with --runner host there.
Interrupting or timing out a container step also signals it inside the container.

The artifact is the tracked files of the revision plus whatever the build tasks
produced, with a manifest recording the revision, the PHP version, the
composer.lock hash and a sha256 list of every file. The deploy job verifies the
revision and compares the PHP version against the target's before publishing, so
a container build records its container's PHP: it passes that check only when the
container runs the target's PHP series.

Exit codes: 0 success, 1 execution failure, 2 usage, 3 missing capability,
4 configuration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployBuild,
}

func init() {
	bindDeployBuildFlags(deployBuildCmd)
	deployCmd.AddCommand(deployBuildCmd)
}

// DeployBuildCommand exposes the build subcommand for tests.
func DeployBuildCommand() *cobra.Command { return deployBuildCmd }

// bindDeployBuildFlags registers the set a build honours. It is narrower than
// the deploy set on purpose: publish strategy, verification and the lock have
// no meaning without a target.
func bindDeployBuildFlags(command *cobra.Command) {
	command.Flags().String("remote", "", "Remote whose configuration the build uses (alternative to the positional argument)")
	command.Flags().String("output", "", "Directory that receives the artifact (required)")
	command.Flags().String("branch", "", "Branch being built")
	command.Flags().String("revision", "", "Exact commit to build (defaults to the local HEAD)")
	command.Flags().String("tag", "", "Tag to build")
	command.Flags().Bool("force", false, "Replace an output directory that is not empty")
	command.Flags().Bool("json", false, "Emit the manifest as machine-readable JSON")
	// No backticks: cobra reads a backquoted word in a usage string as the
	// flag's value placeholder and prints it in place of the flag name.
	command.Flags().String("runner", "host", "Where the build tasks run: host (default, needs nothing) or container (the project's own app container, which has to be running; keep --output inside the project root)")
	command.Flags().Duration(deployFlagTimeout, 0, "Timeout for a single build command")
}

// deployBuildRunner resolves `--runner` into the runner the build tasks execute
// through. The host is the default and the only requirement-free answer: the
// documented build job carries govard and the project's toolchain, and Docker is
// deliberately not among them, so asking for a container runtime on this path
// would take away the surface this command exists to provide.
//
// The container is the way out for a project whose build cannot fetch its own
// dependencies from the host — a private-VCS package whose SSH material the
// container has and the host shell does not. The exec target is the one
// `govard tool php` resolves, so the build runs in the same container, as the
// same account and in the same workdir that command already uses.
func deployBuildRunner(ctx context.Context, config engine.Config, name, workDir, outputDir string) (deploy.Runner, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "host":
		return deploy.LocalRunner{}, nil
	case "container":
		target := resolveToolExecution(config, "php", "")
		runner := deploy.ContainerRunner{
			Container:     target.ContainerName,
			User:          target.User,
			LocalRoot:     workDir,
			ContainerRoot: target.Workdir,
		}
		// The refusal comes before the probe: a --output the container cannot
		// reach is a configuration mistake, and answering it with "install
		// Docker" would send the operator after a daemon that could not have
		// made the path mappable.
		if _, err := runner.ContainerPath(outputDir); err != nil {
			return nil, containerPathConfigError(err)
		}
		if err := requireDocker(containerRunnerHint); err != nil {
			return nil, err
		}
		// The daemon answering says nothing about the project's container. This
		// probe runs before the build clears --output, so a stopped container
		// is exit 3 with the way out, and the previous artifact is still there.
		if err := probeContainerRunning(ctx, runner.Container); err != nil {
			return nil, err
		}
		return runner, nil
	default:
		return nil, &cli.UsageError{Err: fmt.Errorf("--runner must be host or container, got %q", name)}
	}
}

// DeployBuildRunnerForTest exposes the runner resolution for tests.
func DeployBuildRunnerForTest(config engine.Config, name, workDir, outputDir string) (deploy.Runner, error) {
	return deployBuildRunner(context.Background(), config, name, workDir, outputDir)
}

// containerRunnerHint is the way out of every refusal that means "there is no
// container to build in".
const containerRunnerHint = "run `govard env up` first, or build with --runner host"

// probeContainerRunning asks the daemon whether the project container is up. A
// container that is stopped or does not exist is the same missing requirement
// as a missing daemon: exit 3, with the hint that starts it.
func probeContainerRunning(ctx context.Context, container string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	output, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", container).CombinedOutput()
	state := strings.TrimSpace(string(output))
	if err == nil && state == "true" {
		return nil
	}
	detail := fmt.Sprintf("the project container %s is not running", container)
	if err != nil {
		detail = fmt.Sprintf("the project container %s could not be inspected, so it is not running (docker inspect: %s)", container, daemonSentence(state, err))
	}
	if !errorJSON {
		pterm.Info.Printf("Hint: %s\n", containerRunnerHint)
	}
	return &runtime.MissingError{
		Caps:   []runtime.Capability{runtime.CapDocker},
		Detail: detail,
		Hint:   containerRunnerHint,
	}
}

// daemonSentence is the daemon's own sentence, or the exec error when it printed
// nothing.
func daemonSentence(output string, err error) string {
	if line, _, _ := strings.Cut(output, "\n"); strings.TrimSpace(line) != "" {
		return strings.TrimSpace(line)
	}
	return err.Error()
}

// containerPathConfigError marks a path the container cannot reach as the
// configuration mistake it is (exit 4): --output has to move, and nothing the
// runtime could do would make it reachable.
func containerPathConfigError(err error) error {
	if errors.Is(err, deploy.ErrContainerPathUnmappable) {
		return &cli.ConfigError{Err: err}
	}
	return err
}

func runDeployBuild(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	output = strings.TrimSpace(output)
	if output == "" {
		return &cli.UsageError{Err: fmt.Errorf("--output is required: govard deploy build %s --output <dir>", remote)}
	}
	force, _ := cmd.Flags().GetBool("force")
	jsonOut, _ := cmd.Flags().GetBool("json")
	runnerName, _ := cmd.Flags().GetString("runner")

	config, recipe, options, err := resolveDeployRecipeOptions(cmd, remote)
	if err != nil {
		return configOrUsageError(err)
	}
	hooks, err := hooksFromConfig(options.Hooks)
	if err != nil {
		return configOrUsageError(err)
	}

	absolute, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve the output directory %s: %w", output, err)
	}
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve the working directory: %w", err)
	}
	runner, err := deployBuildRunner(cmd.Context(), config, runnerName, workDir, absolute)
	if err != nil {
		return err
	}

	// The build has no target, so its variable set is anchored on the output
	// directory: `{{deploy_path}}` is where the artifact is being assembled,
	// and `deploy:artifact` re-points `{{release_path}}` at the release.
	buildHost := deploy.HostForTest(filepath.Dir(absolute), runner)

	manifest, err := deploy.BuildArtifactDir(cmd.Context(), deploy.BuildRequest{
		Recipe:    recipe,
		Hooks:     hooks,
		Options:   options,
		Vars:      deployVars(buildHost, options),
		WorkDir:   workDir,
		OutputDir: absolute,
		Force:     force,
		Out:       cmd.OutOrStdout(),
		Runner:    runner,
	})
	if err != nil {
		// No current step maps a path the runner check above did not; this
		// guards a future caller that hands the container a new directory.
		return containerPathConfigError(err)
	}

	if jsonOut {
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return fmt.Errorf("encode the artifact manifest: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return nil
	}
	pterm.Success.Printf("Built artifact for %s in %s (%d files, revision %s)\n",
		remote, absolute, manifest.FileCount, deploy.ShortRevision(manifest.Revision))
	return nil
}
