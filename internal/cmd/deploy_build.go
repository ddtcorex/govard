package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
identically.

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
func deployBuildRunner(config engine.Config, name, workDir, outputDir string) (deploy.Runner, error) {
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
			return nil, err
		}
		if err := requireDocker("run `govard env up` first, or build with --runner host"); err != nil {
			return nil, err
		}
		return runner, nil
	default:
		return nil, &cli.UsageError{Err: fmt.Errorf("--runner must be host or container, got %q", name)}
	}
}

// DeployBuildRunnerForTest exposes the runner resolution for tests.
func DeployBuildRunnerForTest(config engine.Config, name, workDir, outputDir string) (deploy.Runner, error) {
	return deployBuildRunner(config, name, workDir, outputDir)
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
	runner, err := deployBuildRunner(config, runnerName, workDir, absolute)
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
		return err
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
