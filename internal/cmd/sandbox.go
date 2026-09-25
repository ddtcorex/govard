package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/cli"
	"govard/internal/conventions"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// sandboxCmd gives a project a real deployment target on this machine: a
// container that plays the remote, reached over real SSH and real rsync.
//
// It is a product command and the deploy feature's regression suite at once.
// Nothing in the pipeline knows it is talking to a sandbox, which is the point:
// a sandbox deploy is a production deploy pointed at a container.
var sandboxCmd = &cobra.Command{
	Annotations: map[string]string{
		// The sandbox needs a container runtime:
		// govard uses it to create the fake server, then talks to it over SSH.
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "sandbox [up|status|reset|ssh|down]",
	Short: "Create a container that plays the deployment target for this project",
	Long: `Create and manage a local deployment target.

` + "`govard sandbox up`" + ` builds a container, publishes SSH on a free loopback
port, generates a dedicated key and resolves as a remote automatically
whenever it is running — no configuration is written anywhere. A deploy to it
uses the same code path as a production deploy: SSH, a git mirror, git archive,
rsync, publish, verify.

The container mounts a mirror of your local repository, refreshed before every
deploy, so a commit you have never pushed is deployable.

Because ` + "`sandbox`" + ` is a top-level command, deploy to it with the flag form:

  govard deploy --remote sandbox --yes

Profiles: basic (sshd, rsync, git), php (adds php-cli, composer, node) and full
(adds a database and a cache). --php picks the PHP series the image provides
(e.g. --php 8.4); without it the sandbox uses the project's stack.php_version
when it names a series, and the base distribution's version otherwise.
The sandbox then declares that series to the pipeline, so a project whose
composer.lock needs a newer PHP can be rehearsed against the PHP its target
actually runs. --docroot shapes the target so the publish strategy resolves the
way you want to exercise it: absent or symlink selects the atomic swap, real
selects in-place publishing.

Exit codes: 0 success, 1 execution failure, 2 usage, 3 missing capability,
4 configuration. The sandbox needs Docker: without a container runtime the
command exits 3 with CAPABILITY_MISSING before creating anything.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// With no subcommand, report: creating a container by accident is a
		// much worse default than printing what is already there.
		return runSandboxStatus(cmd)
	},
}

var (
	sandboxUpCmd     = &cobra.Command{Use: "up", Short: "Create or reuse the sandbox", Long: "Create the sandbox, or reuse the running one as-is. Flags that disagree with a running sandbox: --docroot reshapes only when passed, while an explicit conflicting --profile is refused — pass --recreate to rebuild it.", Args: cobra.NoArgs, RunE: runSandboxUp}
	sandboxStatusCmd = &cobra.Command{Use: "status", Short: "Report the sandbox state", Args: cobra.NoArgs, RunE: runSandboxStatusRun}
	sandboxResetCmd  = &cobra.Command{Use: "reset", Short: "Wipe the sandbox's deploy directories", Args: cobra.NoArgs, RunE: runSandboxReset}
	sandboxSSHCmd    = &cobra.Command{Use: "ssh", Short: "Open a shell in the sandbox", Long: "Open an interactive shell in the sandbox. Takes no command: it always drops into the shell.", Args: cobra.NoArgs, RunE: runSandboxSSH}
	sandboxDownCmd   = &cobra.Command{Use: "down", Short: "Stop and remove the sandbox", Args: cobra.NoArgs, RunE: runSandboxDown}
)

func init() {
	sandboxUpCmd.Flags().String("profile", deploy.DefaultSandboxProfile, "Container contents: basic, php or full")
	sandboxUpCmd.Flags().String("php", "", "PHP series the image provides, e.g. 8.4 (default: stack.php_version when it names a series, else the base image's own; no effect with --profile basic)")
	sandboxUpCmd.Flags().String("docroot", "", "Shape of the target's current path: absent, symlink or real")
	sandboxUpCmd.Flags().Bool("recreate", false, "Rebuild the image and recreate the container")
	sandboxUpCmd.Flags().Bool("no-seed", false, "Skip the snapshot: start with an empty sandbox (no DB, no media, no env file)")

	sandboxResetCmd.Flags().String("docroot", "", "Shape of the target's current path: absent, symlink or real")
	sandboxResetCmd.Flags().String("layout", "", "Seed a target the other deploy tool owns (deployer; any other value is ignored)")

	sandboxDownCmd.Flags().Bool("purge", false, "Also remove the image, the key and the mirror")
	sandboxDownCmd.Flags().Bool("volumes", false, "Also delete the derived data volumes (plain down keeps them so a rehearsal resumes)")

	sandboxCmd.AddCommand(sandboxUpCmd)
	sandboxCmd.AddCommand(sandboxStatusCmd)
	sandboxCmd.AddCommand(sandboxResetCmd)
	sandboxCmd.AddCommand(sandboxSSHCmd)
	sandboxCmd.AddCommand(sandboxDownCmd)
	rootCmd.AddCommand(sandboxCmd)
}

// SandboxCommand exposes the sandbox group for tests.
func SandboxCommand() *cobra.Command { return sandboxCmd }

// SandboxStatusCommand exposes the status subcommand for tests.
func SandboxStatusCommand() *cobra.Command { return sandboxStatusCmd }

// SandboxUpCommand exposes the up subcommand for tests.
func SandboxUpCommand() *cobra.Command { return sandboxUpCmd }

// SandboxResetCommand exposes the reset subcommand for tests.
func SandboxResetCommand() *cobra.Command { return sandboxResetCmd }

// SandboxDownCommand exposes the down subcommand for tests.
func SandboxDownCommand() *cobra.Command { return sandboxDownCmd }

// sandboxCommandRequest builds the library request from the project and the flags.
func sandboxCommandRequest(cmd *cobra.Command) (deploy.SandboxRequest, error) {
	root, err := os.Getwd()
	if err != nil {
		return deploy.SandboxRequest{}, fmt.Errorf("resolve the project directory: %w", err)
	}
	config, err := loadFullConfig()
	if err != nil {
		return deploy.SandboxRequest{}, err
	}

	profile, _ := cmd.Flags().GetString("profile")
	php, _ := cmd.Flags().GetString("php")
	// `--php` has no default, so an unchanged flag is "no preference": the
	// sandbox then builds what the project itself runs, from its normalized
	// stack version. An explicitly named series always wins — including over a
	// reused container's label, through the existing `--recreate` refusal.
	if !cmd.Flags().Changed("php") {
		php = deploy.SandboxPHPDefault(config.Stack.PHPVersion)
	}
	docRoot, _ := cmd.Flags().GetString("docroot")
	layout, _ := cmd.Flags().GetString("layout")
	recreate, _ := cmd.Flags().GetBool("recreate")
	purge, _ := cmd.Flags().GetBool("purge")
	noSeed, _ := cmd.Flags().GetBool("no-seed")
	volumes, _ := cmd.Flags().GetBool("volumes")
	// Naming a shape is what turns "reuse this sandbox" into "lay it out again".
	// The `reset` command overrides this: wiping is what reset does.
	reshapeDocRoot := cmd.Flags().Changed("docroot")
	// `--profile` has a default, so "no preference" and "asked for the default"
	// are the same value; only the second may conflict with an existing container.
	profileExplicit := cmd.Flags().Changed("profile")

	// The framework recipe owns what the container has to provide beyond its
	// profile; the core renders it and never interprets it. The project may
	// extend it: its application's database is not the framework's default in
	// every project, and a rehearsal against the wrong one proves nothing.
	//
	// The settings are validated here as well as in `deploy plan`, so a
	// misspelled key is refused before an image is built rather than after.
	recipe := recipeFor(config)
	if err := deploy.ValidateSettings(recipe, config.Deploy.Settings); err != nil {
		return deploy.SandboxRequest{}, configOrUsageError(err)
	}
	requirements := deploy.SandboxRequirementsWithSettings(recipe.Sandbox, config.Deploy.Settings)
	if err := deploy.ValidateSandboxTools(requirements.Tools); err != nil {
		return deploy.SandboxRequest{}, &cli.ConfigError{Err: err}
	}

	request := deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: config.ProjectName,
		Profile:     profile,
		PHP:         php,
		// Where the web server serves from comes from the project, not from a flag:
		// `stack.web_root` is already the answer for the local environment, and two
		// answers would be one too many.
		WebRoot:         config.Stack.WebRoot,
		DocRoot:         docRoot,
		ReshapeDocRoot:  reshapeDocRoot,
		ProfileExplicit: profileExplicit,
		Layout:          layout,
		Requirements:    requirements,
		Recreate:        recreate,
		Purge:           purge,
		Out:             cmd.OutOrStdout(),
		NoSeed:          noSeed,
		Volumes:         volumes,
		// The snapshot derives from this project: its name for the record and
		// its database for the data. Media, env file and rewrite arrive with
		// the framework wiring; until then those sources stay empty and their
		// steps skip.
		SeedOrigin:        config.ProjectName,
		SeedOriginRunning: originEnvRunning(cmd.Context(), config.ProjectName),
		SeedDBContainer:   dbContainerName(config),
		SeedDBUser:        defaultDBCredentialsForFramework(config.Framework).Username,
		SeedDBPassword:    defaultDBCredentialsForFramework(config.Framework).Password,
		SeedDBName:        defaultDBCredentialsForFramework(config.Framework).Database,
	}
	seedSandboxFramework(config, &request)
	return request, nil
}

// seedSandboxFramework fills the framework-owned half of the snapshot: the app
// container, the media/env paths, the rewriter and the post-import database
// rewrite, all from the framework's registered seed definition — never from a
// per-framework switch here. A framework with no definition seeds the database
// only.
func seedSandboxFramework(config engine.Config, request *deploy.SandboxRequest) {
	definition, ok := engine.SandboxSeedFor(config.Framework)
	if !ok {
		return
	}
	appContainer := config.ProjectName + conventions.PHPSuffix
	shared := deploy.SandboxDefaultPaths().DeployPath + "/shared"
	request.SeedAppContainer = appContainer
	request.EnvRewriter = definition.Rewrite
	request.DBRewrite = definition.DBRewrite
	if definition.MediaPath != "" {
		request.SeedMediaSource = conventions.DefaultWorkDir + "/" + definition.MediaPath
		request.SeedMediaTarget = shared + "/" + definition.MediaPath
	}
	if definition.EnvPath != "" {
		request.SeedEnvSource = conventions.DefaultWorkDir + "/" + definition.EnvPath
		request.SeedEnvTarget = shared + "/" + definition.EnvPath
	}
	// base_url defaults to the sandbox web URL inside the seed run (the port
	// is docker-chosen, so cmd cannot know it); SeedEnvMapping only carries
	// explicit overrides, none today.
	request.SeedEnvMapping = map[string]string{}
}

// originEnvRunning reports whether the origin project's containers are up: the
// seed gate needs it, and a stopped origin is a refusal rather than a silent
// empty sandbox. Unknown (docker unreachable) counts as not running — the
// seed's own error then names the remedy.
func originEnvRunning(ctx context.Context, project string) bool {
	names, err := engine.GetRunningProjectNames(ctx)
	if err != nil {
		return false
	}
	for _, name := range names {
		if name == project {
			return true
		}
	}
	return false
}

func runSandboxUp(cmd *cobra.Command, _ []string) error {
	request, err := sandboxCommandRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxUp(cmd.Context(), deploy.NewDockerCLI(), deploy.LocalRunner{}, request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox ready")
	pterm.Info.Printf("Deploy to it with: govard deploy --remote %s --yes\n", state.RemoteName)
	return nil
}

func runSandboxStatus(cmd *cobra.Command) error {
	return runSandboxStatusRun(cmd, nil)
}

func runSandboxStatusRun(cmd *cobra.Command, _ []string) error {
	request, err := sandboxCommandRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxStatus(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox")
	if !state.Exists {
		pterm.Info.Println("Create it with: govard sandbox up")
	}
	return nil
}

func runSandboxReset(cmd *cobra.Command, _ []string) error {
	request, err := sandboxCommandRequest(cmd)
	if err != nil {
		return err
	}
	// `reset` wipes the deploy directories and lays the target out again: shaping
	// the current path is what it is for, whether or not a shape was named.
	request.ReshapeDocRoot = true
	state, err := deploy.SandboxReset(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox reset")
	return nil
}

func runSandboxDown(cmd *cobra.Command, _ []string) error {
	request, err := sandboxCommandRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxDown(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox removed")
	if request.Purge {
		pterm.Info.Println("The image, the key and the mirror were removed too")
	}
	return nil
}

func runSandboxSSH(cmd *cobra.Command, _ []string) error {
	request, err := sandboxCommandRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxStatus(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Running {
		return &cli.UsageError{Err: fmt.Errorf("the sandbox is not running; run `govard sandbox up` first")}
	}
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh binary not found: %w", err)
	}
	// The session replaces this process, exactly like `govard remote exec`: an
	// interactive shell that govard proxies would lose job control and the TTY.
	args := append([]string{sshPath}, deploy.SandboxSSHArgs(request, state)...)
	return engine.Handoff(sshPath, args)
}

func printSandboxState(cmd *cobra.Command, state *deploy.SandboxState, headline string) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s\n", headline)
	fmt.Fprintf(out, "  container:  %s\n", state.Container)
	if state.Profile != "" {
		fmt.Fprintf(out, "  profile:    %s\n", state.Profile)
	}
	if state.WebPort > 0 {
		fmt.Fprintf(out, "  web:        http://127.0.0.1:%d/\n", state.WebPort)
	}
	if state.PHP != "" {
		fmt.Fprintf(out, "  php:        %s\n", state.PHP)
	}
	if state.Image != "" {
		fmt.Fprintf(out, "  image:      %s\n", state.Image)
	}
	fmt.Fprintf(out, "  running:    %t\n", state.Running)
	if state.Port > 0 {
		fmt.Fprintf(out, "  ssh:        ssh -p %d %s@127.0.0.1\n", state.Port, deploy.SandboxUser)
	}
	fmt.Fprintf(out, "  remote:     %s (%s)\n", state.RemoteName, sandboxRemoteState(state))
	fmt.Fprintf(out, "  deploy:     %s\n", state.DeployPath)
	fmt.Fprintf(out, "  current:    %s\n", state.CurrentPath)
	fmt.Fprintf(out, "  mirror:     %s\n", state.MirrorPath)
	if strings.TrimSpace(state.Packages) != "" {
		fmt.Fprintf(out, "  packages:   %s\n", strings.ReplaceAll(strings.TrimSpace(state.Packages), "\n", ", "))
	}
}

func sandboxRemoteState(state *deploy.SandboxState) string {
	if state.RemoteSet {
		return "configured"
	}
	return "not configured"
}
