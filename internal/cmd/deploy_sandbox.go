package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// deploySandboxCmd gives a project a real deployment target on this machine: a
// container that plays the remote, reached over real SSH and real rsync.
//
// It is a product command and the deploy feature's regression suite at once.
// Nothing in the pipeline knows it is talking to a sandbox, which is the point:
// a sandbox deploy is a production deploy pointed at a container.
var deploySandboxCmd = &cobra.Command{
	Annotations: map[string]string{
		// The only command in the deploy group that needs a container runtime:
		// govard uses it to create the fake server, then talks to it over SSH.
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "sandbox [up|status|reset|ssh|down]",
	Short: "Create a container that plays the deployment target for this project",
	Long: `Create and manage a local deployment target.

` + "`govard deploy sandbox up`" + ` builds a container, publishes SSH on a free loopback
port, generates a dedicated key and writes a ` + "`sandbox`" + ` remote into
.govard.local.yml (local-only, gitignored). A deploy to it uses the same code
path as a production deploy: SSH, a git mirror, git archive, rsync, publish,
verify.

The container mounts a mirror of your local repository, refreshed before every
deploy, so a commit you have never pushed is deployable.

Because ` + "`sandbox`" + ` is a subcommand here, deploy to it with the flag form:

  govard deploy --remote sandbox --yes

Profiles: basic (sshd, rsync, git), php (adds php-cli, composer, node) and full
(adds a database and a cache). --php picks the PHP series the image provides
(e.g. --php 8.4); without it the image keeps the base distribution's version.
The sandbox then declares that series to the pipeline, so a project whose
composer.lock needs a newer PHP can be rehearsed against the PHP its target
actually runs. --docroot shapes the target so the publish strategy resolves the
way you want to exercise it: absent or symlink selects the atomic swap, real
selects in-place publishing.

Exit codes: 0 success, 1 execution failure, 2 usage, 3 missing capability,
4 configuration.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// With no subcommand, report: creating a container by accident is a
		// much worse default than printing what is already there.
		return runDeploySandboxStatus(cmd)
	},
}

var (
	deploySandboxUpCmd     = &cobra.Command{Use: "up", Short: "Create or reuse the sandbox", Args: cobra.NoArgs, RunE: runDeploySandboxUp}
	deploySandboxStatusCmd = &cobra.Command{Use: "status", Short: "Report the sandbox state", Args: cobra.NoArgs, RunE: runDeploySandboxStatusRun}
	deploySandboxResetCmd  = &cobra.Command{Use: "reset", Short: "Wipe the sandbox's deploy directories", Args: cobra.NoArgs, RunE: runDeploySandboxReset}
	deploySandboxSSHCmd    = &cobra.Command{Use: "ssh", Short: "Open a shell in the sandbox", Args: cobra.NoArgs, RunE: runDeploySandboxSSH}
	deploySandboxDownCmd   = &cobra.Command{Use: "down", Short: "Stop and remove the sandbox", Args: cobra.NoArgs, RunE: runDeploySandboxDown}
)

func init() {
	deploySandboxUpCmd.Flags().String("profile", deploy.DefaultSandboxProfile, "Container contents: basic, php or full")
	deploySandboxUpCmd.Flags().String("php", "", "PHP series the image provides, e.g. 8.4 (default: the base image's own)")
	deploySandboxUpCmd.Flags().String("docroot", "", "Shape of the target's current path: absent, symlink or real")
	deploySandboxUpCmd.Flags().Bool("recreate", false, "Rebuild the image and recreate the container")

	deploySandboxResetCmd.Flags().String("docroot", "", "Shape of the target's current path: absent, symlink or real")
	deploySandboxResetCmd.Flags().String("layout", "", "Seed a target the other deploy tool owns: deployer")

	deploySandboxDownCmd.Flags().Bool("purge", false, "Also remove the image, the key and the mirror")

	deploySandboxCmd.AddCommand(deploySandboxUpCmd)
	deploySandboxCmd.AddCommand(deploySandboxStatusCmd)
	deploySandboxCmd.AddCommand(deploySandboxResetCmd)
	deploySandboxCmd.AddCommand(deploySandboxSSHCmd)
	deploySandboxCmd.AddCommand(deploySandboxDownCmd)
	deployCmd.AddCommand(deploySandboxCmd)
}

// DeploySandboxCommand exposes the sandbox group for tests.
func DeploySandboxCommand() *cobra.Command { return deploySandboxCmd }

// DeploySandboxStatusCommand exposes the status subcommand for tests.
func DeploySandboxStatusCommand() *cobra.Command { return deploySandboxStatusCmd }

// DeploySandboxUpCommand exposes the up subcommand for tests.
func DeploySandboxUpCommand() *cobra.Command { return deploySandboxUpCmd }

// DeploySandboxResetCommand exposes the reset subcommand for tests.
func DeploySandboxResetCommand() *cobra.Command { return deploySandboxResetCmd }

// DeploySandboxDownCommand exposes the down subcommand for tests.
func DeploySandboxDownCommand() *cobra.Command { return deploySandboxDownCmd }

// deploySandboxRequest builds the library request from the project and the flags.
func deploySandboxRequest(cmd *cobra.Command) (deploy.SandboxRequest, error) {
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
	docRoot, _ := cmd.Flags().GetString("docroot")
	layout, _ := cmd.Flags().GetString("layout")
	recreate, _ := cmd.Flags().GetBool("recreate")
	purge, _ := cmd.Flags().GetBool("purge")

	// The framework recipe owns what the container has to provide beyond its
	// profile; the core renders it and never interprets it.
	requirements := recipeFor(config).Sandbox

	return deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: config.ProjectName,
		Profile:     profile,
		PHP:         php,
		// Where the web server serves from comes from the project, not from a flag:
		// `stack.web_root` is already the answer for the local environment, and two
		// answers would be one too many.
		WebRoot:      config.Stack.WebRoot,
		DocRoot:      docRoot,
		Layout:       layout,
		Requirements: requirements,
		Recreate:     recreate,
		Purge:        purge,
		Out:          cmd.OutOrStdout(),
	}, nil
}

func runDeploySandboxUp(cmd *cobra.Command, _ []string) error {
	request, err := deploySandboxRequest(cmd)
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

func runDeploySandboxStatus(cmd *cobra.Command) error {
	return runDeploySandboxStatusRun(cmd, nil)
}

func runDeploySandboxStatusRun(cmd *cobra.Command, _ []string) error {
	request, err := deploySandboxRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxStatus(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox")
	if !state.Exists {
		pterm.Info.Println("Create it with: govard deploy sandbox up")
	}
	return nil
}

func runDeploySandboxReset(cmd *cobra.Command, _ []string) error {
	request, err := deploySandboxRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxReset(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	printSandboxState(cmd, state, "Sandbox reset")
	return nil
}

func runDeploySandboxDown(cmd *cobra.Command, _ []string) error {
	request, err := deploySandboxRequest(cmd)
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

func runDeploySandboxSSH(cmd *cobra.Command, _ []string) error {
	request, err := deploySandboxRequest(cmd)
	if err != nil {
		return err
	}
	state, err := deploy.SandboxStatus(cmd.Context(), deploy.NewDockerCLI(), request)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Running {
		return &cli.UsageError{Err: fmt.Errorf("the sandbox is not running; run `govard deploy sandbox up` first")}
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
