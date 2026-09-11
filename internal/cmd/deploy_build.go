package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// deployBuildCmd is the CI half of the artifact mode: it runs where the
// project's toolchain lives and produces the directory `govard deploy
// --artifact-dir` consumes.
var deployBuildCmd = &cobra.Command{
	Annotations: map[string]string{
		// No ssh, no rsync, no container runtime: a build is local work.
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

The artifact is the tracked files of the revision plus whatever the build tasks
produced, with a manifest recording the revision, the PHP version, the
composer.lock hash and a sha256 list of every file. The deploy job verifies the
revision and compares the PHP version against the target's before publishing.

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
	command.Flags().Duration(deployFlagTimeout, 0, "Timeout for a single build command")
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

	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	hooks, err := hooksFromConfig(options.Hooks)
	if err != nil {
		return &cli.UsageError{Err: err}
	}
	recipe, options := deployRecipe(config, options)

	absolute, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve the output directory %s: %w", output, err)
	}
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve the working directory: %w", err)
	}

	// The build has no target, so its variable set is anchored on the output
	// directory: `{{deploy_path}}` is where the artifact is being assembled,
	// and `deploy:artifact` re-points `{{release_path}}` at the release.
	buildHost := deploy.HostForTest(filepath.Dir(absolute), deploy.LocalRunner{})

	manifest, err := deploy.BuildArtifactDir(cmd.Context(), deploy.BuildRequest{
		Recipe:    recipe,
		Hooks:     hooks,
		Options:   options,
		Vars:      deployVars(buildHost, options),
		WorkDir:   workDir,
		OutputDir: absolute,
		Force:     force,
		Out:       cmd.OutOrStdout(),
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
		remote, absolute, manifest.FileCount, shortRevisionForOutput(manifest.Revision))
	return nil
}
