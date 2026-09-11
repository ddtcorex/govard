package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// deployFlagTimeout is the CLI spelling of the per-command timeout.
const deployFlagTimeout = "command-timeout"

// bindDeployFlags registers the deploy command's flags. The set is deliberately
// closed: a flag exists here only if the engine honours it.
func bindDeployFlags(command *cobra.Command) {
	bindDeploySourceFlags(command)
	command.Flags().String("build", deploy.BuildAuto, "Where the build runs: auto, server or artifact")
	command.Flags().String("artifact-dir", "", "Artifact directory built by govard deploy build (implies --build=artifact)")
	command.Flags().String("publish", deploy.PublishAuto, "Publish strategy: auto, symlink or in_place")
	command.Flags().Int("keep", 0, "How many releases to keep on the target")
	negatableBool(command, "verify", true, "Verify the target after publishing")
	negatableBool(command, "db-backup", false, "Dump the database before the first database-mutating task")
	negatableBool(command, "lock", true, "Take the deploy lock")
	command.Flags().Bool("ignore-deployer-lock", false, "Run even when another deploy tool holds its lock")
	command.Flags().Duration(deployFlagTimeout, 0, "Timeout for a single remote command")
	command.Flags().Bool("force", false, "Deploy even when the target already runs this revision")
	command.Flags().Bool("resume", false, "Continue the newest unfinished release instead of starting a new one")
	command.Flags().String("from", "", "Start at this task id or hook name instead of at the beginning")
	command.Flags().Bool("yes", false, "Do not prompt for confirmation")
	command.Flags().Bool("json", false, "Emit machine-readable output")
	command.Flags().Bool("verbose", false, "Stream remote command output")
}

// bindDeploySourceFlags registers the flags that select what is deployed and how
// it is built. Reading commands (`plan`) and preflight (`check`) declare the same
// set: a plan that cannot express the mode it is planning is a plan of a
// different run.
func bindDeploySourceFlags(command *cobra.Command) {
	command.Flags().String("remote", "", "Remote environment to deploy (alternative to the positional argument)")
	command.Flags().String("branch", "", "Branch to deploy (overrides the remote's configured branch)")
	command.Flags().String("revision", "", "Exact commit to deploy")
	command.Flags().String("tag", "", "Tag to deploy")
}

// negatableBool registers a boolean flag together with the `--no-<name>` spelling
// the CLI reference documents.
//
// pflag has no built-in negation: `--no-verify` would be an unknown flag, which
// is what the docs promised it was not. The negated twin is a separate flag
// resolved by negatedBool, so `--verify=false` and `--no-verify` stay the same
// request expressed twice.
func negatableBool(command *cobra.Command, name string, value bool, usage string) {
	command.Flags().Bool(name, value, usage)
	command.Flags().Bool("no-"+name, false, "Disable --"+name+" (same as --"+name+"=false)")
}

// negatedBool resolves a boolean flag and its `--no-<name>` twin.
//
// A contradiction — asking for both values at once — is refused rather than
// resolved by precedence: one of the two was not what the operator meant, and
// guessing which would make a deploy do the opposite of the request.
func negatedBool(flags *pflag.FlagSet, name string) (bool, error) {
	value, _ := flags.GetBool(name)
	negation := flags.Lookup("no-" + name)
	if negation == nil || !negation.Changed {
		return value, nil
	}
	if flags.Changed(name) && value {
		return false, fmt.Errorf("--%s and --no-%s contradict each other", name, name)
	}
	return false, nil
}

// overridesFromFlags reads the flag values.
func overridesFromFlags(command *cobra.Command) (deploy.Overrides, error) {
	flags := command.Flags()

	remote, _ := flags.GetString("remote")
	branch, _ := flags.GetString("branch")
	revision, _ := flags.GetString("revision")
	tag, _ := flags.GetString("tag")
	build, _ := flags.GetString("build")
	artifactDir, _ := flags.GetString("artifact-dir")
	publish, _ := flags.GetString("publish")
	keep, _ := flags.GetInt("keep")
	ignoreDeployerLock, _ := flags.GetBool("ignore-deployer-lock")
	timeout, _ := flags.GetDuration(deployFlagTimeout)
	force, _ := flags.GetBool("force")
	resume, _ := flags.GetBool("resume")
	from, _ := flags.GetString("from")
	yes, _ := flags.GetBool("yes")
	jsonOut, _ := flags.GetBool("json")
	verbose, _ := flags.GetBool("verbose")

	over := deploy.Overrides{
		Branch:             strings.TrimSpace(branch),
		Revision:           strings.TrimSpace(revision),
		Tag:                strings.TrimSpace(tag),
		Build:              strings.TrimSpace(build),
		ArtifactDir:        strings.TrimSpace(artifactDir),
		Publish:            strings.TrimSpace(publish),
		KeepReleases:       keep,
		IgnoreDeployerLock: ignoreDeployerLock,
		CommandTimeout:     timeout,
		Resume:             resume,
		From:               strings.TrimSpace(from),
		Force:              force,
		Yes:                yes,
		JSON:               jsonOut,
		Verbose:            verbose,
	}
	_ = remote

	// The three-state options are read only when the command actually declares
	// their flag. A subcommand (`deploy plan`, `deploy check`, `deploy unlock`)
	// deliberately declares a narrower set, and "the flag is absent" must not be
	// read as "the operator turned it off".
	for _, name := range []string{"verify", "db-backup", "lock"} {
		if flags.Lookup(name) == nil {
			continue
		}
		value, err := negatedBool(flags, name)
		if err != nil {
			return deploy.Overrides{}, &cli.UsageError{Err: err}
		}
		switch name {
		case "verify":
			over.Verify = &value
		case "db-backup":
			over.DBBackup = &value
		case "lock":
			over.Lock = &value
		}
	}

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
	default:
		return fmt.Errorf("--publish must be %s, %s or %s", deploy.PublishAuto, deploy.PublishSymlink, deploy.PublishInPlace)
	}
	// Only the value is checked here: `--build=artifact` with no flag is legal
	// when the project sets deploy.artifact_dir, and that is resolved after the
	// configuration is loaded.
	switch strings.ToLower(over.Build) {
	case "", deploy.BuildAuto, deploy.BuildServer, deploy.BuildArtifact:
		return nil
	default:
		return fmt.Errorf("--build must be %s, %s or %s", deploy.BuildAuto, deploy.BuildServer, deploy.BuildArtifact)
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
//
// A flag problem is a usage error and a `.govard.yml` problem is a configuration
// error: the exit code tells an operator which file to open, and CI which class
// of failure it is looking at.
func resolveDeployOptions(command *cobra.Command, remote string) (engine.Config, deploy.Options, error) {
	// Flag validation comes first: a bad flag combination is a usage error even
	// when the project config is missing or unreadable.
	over, err := overridesFromFlags(command)
	if err != nil {
		return engine.Config{}, deploy.Options{}, err
	}
	config, err := loadFullConfig()
	if err != nil {
		return engine.Config{}, deploy.Options{}, &cli.ConfigError{Err: err}
	}
	options, err := deploy.ResolveOptions(config, remote, over)
	if err != nil {
		if errors.Is(err, deploy.ErrInvalidConfiguration) {
			return engine.Config{}, deploy.Options{}, &cli.ConfigError{Err: err}
		}
		return engine.Config{}, deploy.Options{}, &cli.UsageError{Err: err}
	}
	return config, options, nil
}

// deployHostFor resolves the target host for a remote and, when the remote does
// not configure a deploy path, adopts the layout the server already has.
//
// `deploy_path` has no safe default, so an empty one cannot simply be joined
// into paths — that would silently address a directory nobody chose. It is
// instead read from the server (spec 5.1), adopted only when the answer is
// unambiguous, and reported so the operator can make it explicit. A remote with
// no discoverable layout is a configuration error: the fix is a file edit.
func deployHostFor(ctx context.Context, config engine.Config, remote string, options deploy.Options, out io.Writer) (deploy.Host, error) {
	host, err := deploy.HostForConfig(config, remote, options)
	if err != nil {
		return deploy.Host{}, err
	}
	if strings.TrimSpace(host.DeployPath) != "" {
		return host, nil
	}

	discovered, err := deploy.DiscoverDeployPath(ctx, host)
	if err != nil {
		return deploy.Host{}, &cli.ConfigError{Err: fmt.Errorf("remote %q: %w", remote, err)}
	}
	host.DeployPath = discovered
	if out != nil {
		fmt.Fprintf(out, "using the deploy layout already on %s: %s (set remotes.%s.deploy_path to make it explicit)\n",
			remote, discovered, remote)
	}
	return host, nil
}

// configOrUsageError classifies an error that comes from a configuration file
// the plan builder refused — an unknown hook anchor, a duplicate hook name, a
// cycle. The remedy is always an edit to `.govard.yml`, never to the command.
func configOrUsageError(err error) error {
	switch {
	case errors.Is(err, deploy.ErrUnknownAnchor),
		errors.Is(err, deploy.ErrDuplicateHook),
		errors.Is(err, deploy.ErrHookCycle),
		errors.Is(err, deploy.ErrInvalidConfiguration):
		return &cli.ConfigError{Err: err}
	default:
		return &cli.UsageError{Err: err}
	}
}

// recipeFor returns the deploy recipe for the project's framework.
//
// This is the single seam where a framework recipe enters the pipeline: the
// registry owns the recipe, internal/deploy never imports it, and a framework
// without one still gets the neutral default pipeline.
func recipeFor(config engine.Config) deploy.Recipe {
	if recipe, ok := frameworks.DeployRecipe(config.Framework); ok {
		return recipe
	}
	return deploy.DefaultRecipe()
}

// deployRecipe resolves the recipe, validates the project's settings against it,
// and layers the recipe's defaults under the options — so `plan`, `check`,
// `build` and `deploy` can never disagree about the values a command expands
// with, and a setting the recipe does not know is refused before anything runs
// (spec 5.2, exit 4).
//
// The validation sees the settings as the operator wrote them: the recipe's own
// defaults are layered afterwards, so a key govard invented is never mistaken for
// one a project configured.
func deployRecipe(config engine.Config, options deploy.Options) (deploy.Recipe, deploy.Options, error) {
	recipe := recipeFor(config)
	if err := deploy.ValidateSettings(recipe, options.Settings); err != nil {
		return deploy.Recipe{}, deploy.Options{}, err
	}
	return recipe, deploy.WithRecipeDefaults(recipe, options), nil
}

// deployPlanFor composes the recipe with the project's hooks and shapes the
// result for the resolved build mode. `plan` and `deploy` share it, so the tree
// an operator reviews is the tree the executor runs.
func deployPlanFor(recipe deploy.Recipe, hooks []deploy.Hook, remote string, options deploy.Options) (deploy.Plan, error) {
	plan, err := deploy.BuildPlan(recipe, hooks, remote)
	if err != nil {
		return deploy.Plan{}, err
	}
	return plan.ForBuildMode(options.Build).ForPublishStrategy(options.Publish), nil
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
		if text, ok := settingText(value); ok {
			vars = vars.Set("settings."+key, text)
		}
	}
	// The argument lists a recipe rendered are substituted verbatim: quoting
	// them would collapse several arguments into one.
	for key, value := range options.Settings {
		if text, ok := settingText(value); ok && strings.HasSuffix(key, "_args") {
			vars = vars.SetRaw("settings."+key, text)
		}
	}
	// Deployer's content version was a timestamp, which busts every browser
	// cache on every deploy. The revision is deterministic: a retry or a resume
	// of the same revision writes the same static URLs, and a new revision
	// still changes them. A project can still override it.
	if text, ok := options.Settings["content_version"].(string); ok && strings.TrimSpace(text) == "" {
		// Short, per spec 10.4: readable in an asset URL, and still different
		// for every revision so a deploy busts caches exactly once.
		vars = vars.Set("settings.content_version", deploy.ShortRevision(options.Revision))
	}
	return vars
}

// settingText renders a setting as the string a command template substitutes.
// Booleans and numbers are rendered too: a recipe that guards a step with
// `[ "{{settings.worker_control}}" = "true" ]` must not fail with "unknown
// variable" just because the configuration expressed the value as a bool.
func settingText(value any) (string, bool) {
	return deploy.SettingText(value)
}

// DeployVarsForTest exposes deployVars to the tests/ package.
func DeployVarsForTest(host deploy.Host, options deploy.Options) deploy.Vars {
	return deployVars(host, options)
}
