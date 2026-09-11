package cmd

import (
	"fmt"
	"strings"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

// deployFlagTimeout is the CLI spelling of the per-command timeout.
const deployFlagTimeout = "command-timeout"

// bindDeployFlags registers the deploy command's flags. The set is deliberately
// closed: a flag exists here only if the engine honours it.
func bindDeployFlags(command *cobra.Command) {
	command.Flags().String("remote", "", "Remote environment to deploy (alternative to the positional argument)")
	command.Flags().String("branch", "", "Branch to deploy (overrides the remote's configured branch)")
	command.Flags().String("revision", "", "Exact commit to deploy")
	command.Flags().String("tag", "", "Tag to deploy")
	command.Flags().String("publish", deploy.PublishAuto, "Publish strategy: auto, symlink or in_place")
	command.Flags().Int("keep", 0, "How many releases to keep on the target")
	command.Flags().Bool("verify", true, "Verify the target after publishing")
	command.Flags().Bool("lock", true, "Take the deploy lock")
	command.Flags().Bool("ignore-deployer-lock", false, "Run even when another deploy tool holds its lock")
	command.Flags().Duration(deployFlagTimeout, 0, "Timeout for a single remote command")
	command.Flags().Bool("force", false, "Deploy even when the target already runs this revision")
	command.Flags().Bool("resume", false, "Continue the newest unfinished release instead of starting a new one")
	command.Flags().Bool("yes", false, "Do not prompt for confirmation")
	command.Flags().Bool("json", false, "Emit machine-readable output")
	command.Flags().Bool("verbose", false, "Stream remote command output")
}

// overridesFromFlags reads the flag values.
func overridesFromFlags(command *cobra.Command) (deploy.Overrides, error) {
	flags := command.Flags()

	remote, _ := flags.GetString("remote")
	branch, _ := flags.GetString("branch")
	revision, _ := flags.GetString("revision")
	tag, _ := flags.GetString("tag")
	publish, _ := flags.GetString("publish")
	keep, _ := flags.GetInt("keep")
	verify, _ := flags.GetBool("verify")
	lock, _ := flags.GetBool("lock")
	ignoreDeployerLock, _ := flags.GetBool("ignore-deployer-lock")
	timeout, _ := flags.GetDuration(deployFlagTimeout)
	force, _ := flags.GetBool("force")
	resume, _ := flags.GetBool("resume")
	yes, _ := flags.GetBool("yes")
	jsonOut, _ := flags.GetBool("json")
	verbose, _ := flags.GetBool("verbose")

	over := deploy.Overrides{
		Branch:             strings.TrimSpace(branch),
		Revision:           strings.TrimSpace(revision),
		Tag:                strings.TrimSpace(tag),
		Publish:            strings.TrimSpace(publish),
		KeepReleases:       keep,
		Verify:             &verify,
		Lock:               &lock,
		IgnoreDeployerLock: ignoreDeployerLock,
		CommandTimeout:     timeout,
		Resume:             resume,
		Force:              force,
		Yes:                yes,
		JSON:               jsonOut,
		Verbose:            verbose,
	}
	_ = remote

	// A flag combination the engine would have to guess about is a usage error,
	// not something to resolve silently.
	if err := validateDeployFlagCombination(over); err != nil {
		return deploy.Overrides{}, &cli.UsageError{Err: err}
	}
	if timeout < 0 {
		return deploy.Overrides{}, &cli.UsageError{Err: fmt.Errorf("--%s must not be negative", deployFlagTimeout)}
	}
	return over, nil
}

func validateDeployFlagCombination(over deploy.Overrides) error {
	chosen := 0
	for _, value := range []string{over.Branch, over.Revision, over.Tag} {
		if value != "" {
			chosen++
		}
	}
	if chosen > 1 {
		return fmt.Errorf("--branch, --revision and --tag are mutually exclusive")
	}
	switch over.Publish {
	case "", deploy.PublishAuto, deploy.PublishSymlink, deploy.PublishInPlace:
		return nil
	default:
		return fmt.Errorf("--publish must be %s, %s or %s", deploy.PublishAuto, deploy.PublishSymlink, deploy.PublishInPlace)
	}
}

// DeployOverridesForTest parses a deploy flag list and returns the resolved
// overrides. Tests use it instead of cobra's Execute(), which re-targets a
// subcommand through the root command and never reaches RunE.
func DeployOverridesForTest(args []string) (deploy.Overrides, error) {
	command := &cobra.Command{Use: "deploy", RunE: func(*cobra.Command, []string) error { return nil }}
	bindDeployFlags(command)
	if err := command.ParseFlags(args); err != nil {
		return deploy.Overrides{}, &cli.UsageError{Err: err}
	}
	return overridesFromFlags(command)
}

// deployRemoteName resolves the target remote from the positional argument or
// the flag, so a remote whose name collides with a subcommand is still usable.
func deployRemoteName(command *cobra.Command, args []string) (string, error) {
	fromFlag, _ := command.Flags().GetString("remote")
	fromFlag = strings.TrimSpace(fromFlag)
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		positional := strings.TrimSpace(args[0])
		if fromFlag != "" && fromFlag != positional {
			return "", &cli.UsageError{Err: fmt.Errorf("remote %q does not match --remote %q", positional, fromFlag)}
		}
		return positional, nil
	}
	if fromFlag != "" {
		return fromFlag, nil
	}
	return "", &cli.UsageError{Err: fmt.Errorf("a remote is required: govard deploy <remote>")}
}

// resolveDeployOptions loads the project config and resolves the effective
// options for one remote.
func resolveDeployOptions(command *cobra.Command, remote string) (engine.Config, deploy.Options, error) {
	// Flag validation comes first: a bad flag combination is a usage error even
	// when the project config is missing or unreadable.
	over, err := overridesFromFlags(command)
	if err != nil {
		return engine.Config{}, deploy.Options{}, err
	}
	config, err := loadFullConfig()
	if err != nil {
		return engine.Config{}, deploy.Options{}, err
	}
	options, err := deploy.ResolveOptions(config, remote, over)
	if err != nil {
		return engine.Config{}, deploy.Options{}, &cli.UsageError{Err: err}
	}
	return config, options, nil
}

// recipeFor returns the deploy recipe for the project's framework.
//
// This is the single seam where a framework recipe enters the pipeline: PR 1
// returns the neutral default recipe, and a framework recipe is looked up in
// the framework registry here once it exists.
func recipeFor(config engine.Config) deploy.Recipe {
	_ = config
	return deploy.DefaultRecipe()
}

// hooksFromConfig converts the project's deploy hooks, reporting the first
// invalid one as a configuration error rather than ignoring it.
func hooksFromConfig(configs []engine.DeployHookConfig) ([]deploy.Hook, error) {
	hooks := make([]deploy.Hook, 0, len(configs))
	for _, cfg := range configs {
		hook, err := deploy.HookFromConfig(cfg)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, hook)
	}
	return hooks, nil
}

// deployVars builds the variable set available to recipe commands and hooks.
func deployVars(host deploy.Host, options deploy.Options) deploy.Vars {
	vars := deploy.NewVars().
		SetPath("release_path", "").
		SetPath("previous_release", "").
		SetPath("deploy_path", host.DeployPath).
		SetPath("current_path", host.CurrentPath).
		SetPath("shared_path", host.SharedPath()).
		SetPath("repo_path", host.RepoPath()).
		Set("release", "").
		Set("branch", options.Branch).
		Set("revision", options.Revision).
		Set("tag", options.Tag).
		Set("verify_url", options.VerifyURL)

	if entry, ok := options.Settings["php_bin"].(string); ok {
		vars = vars.Set("php_bin", entry)
	} else {
		vars = vars.Set("php_bin", "php")
	}
	if entry, ok := options.Settings["composer_bin"].(string); ok {
		vars = vars.Set("composer_bin", entry)
	} else {
		vars = vars.Set("composer_bin", "composer")
	}
	for key, value := range options.Settings {
		if text, ok := value.(string); ok {
			vars = vars.Set("settings."+key, text)
		}
	}
	return vars
}
