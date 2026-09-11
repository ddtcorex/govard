// Package deploy implements Govard's deployment pipeline: a neutral, ordered
// task model, the plan that composes a framework recipe with project hooks, and
// the executor that runs each step on a target host.
//
// The package is deliberately framework-agnostic. It never branches on a
// framework name: a framework contributes behaviour by returning a Recipe, and
// a project contributes behaviour by anchoring hooks on task ids, stage aliases
// or other hooks.
package deploy

import (
	"fmt"
	"strings"
	"time"

	"govard/internal/engine"
)

// Publish strategies. A remote may declare one explicitly; `auto` lets the
// engine decide from the layout the server actually has.
const (
	PublishAuto    = "auto"
	PublishSymlink = "symlink"
	PublishInPlace = "in_place"
)

// Defaults applied when neither the project nor the remote configures a value.
const (
	DefaultCommandTimeout = 30 * time.Minute
	DefaultVerifyTimeout  = 30 * time.Second
)

// Overrides carries the CLI flag values for one invocation. Zero values mean
// "not set", so a flag can always be distinguished from an explicit default.
type Overrides struct {
	Branch             string
	Revision           string
	Tag                string
	Publish            string
	KeepReleases       int
	Verify             *bool
	Lock               *bool
	IgnoreDeployerLock bool
	CommandTimeout     time.Duration
	Resume             bool
	Force              bool
	Yes                bool
	JSON               bool
	Verbose            bool
}

// Options is the fully resolved configuration for one deploy run: recipe
// defaults, then the project deploy block, then the per-remote override block,
// then the flags.
type Options struct {
	Remote             string
	Branch             string
	Revision           string
	Tag                string
	Repository         string
	Publish            string
	KeepReleases       int
	Verify             bool
	VerifyURL          string
	VerifyTimeout      time.Duration
	Lock               bool
	IgnoreDeployerLock bool
	CommandTimeout     time.Duration
	Resume             bool
	Force              bool
	Yes                bool
	JSON               bool
	Verbose            bool
	Settings           map[string]any
	Hooks              []engine.DeployHookConfig
}

// ResolveOptions resolves one remote's effective deploy options.
func ResolveOptions(cfg engine.Config, remote string, over Overrides) (Options, error) {
	name := strings.ToLower(strings.TrimSpace(remote))
	remoteCfg, ok := cfg.Remotes[name]
	if !ok {
		return Options{}, errUnknownRemote(remote, cfg)
	}

	effective := cfg.Deploy
	if remoteCfg.Deploy != nil {
		effective = mergeDeployConfig(cfg.Deploy, *remoteCfg.Deploy)
	}

	opts := Options{
		Remote:         name,
		Branch:         remoteCfg.Branch,
		Repository:     remoteCfg.Repository,
		Publish:        remoteCfg.Publish,
		KeepReleases:   effective.KeepReleasesOr(),
		Verify:         true,
		Lock:           true,
		VerifyURL:      effective.Verify.URL,
		VerifyTimeout:  DefaultVerifyTimeout,
		CommandTimeout: DefaultCommandTimeout,
		Settings:       mergedSettings(effective.Settings),
		Hooks:          effective.Hooks,
	}

	if raw := strings.TrimSpace(effective.Verify.Timeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Options{}, fmt.Errorf("deploy.verify.timeout: %w", err)
		}
		opts.VerifyTimeout = parsed
	}
	if raw := strings.TrimSpace(effective.CommandTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Options{}, fmt.Errorf("deploy.command_timeout: %w", err)
		}
		opts.CommandTimeout = parsed
	}

	// Flags win over every configuration layer.
	if over.Branch != "" {
		opts.Branch = over.Branch
	}
	opts.Revision, opts.Tag = over.Revision, over.Tag
	if over.Publish != "" {
		opts.Publish = over.Publish
	}
	if opts.Publish == "" {
		opts.Publish = PublishAuto
	}
	if over.KeepReleases > 0 {
		opts.KeepReleases = over.KeepReleases
	}
	if over.Verify != nil {
		opts.Verify = *over.Verify
	}
	if over.Lock != nil {
		opts.Lock = *over.Lock
	}
	opts.IgnoreDeployerLock = over.IgnoreDeployerLock
	if over.CommandTimeout > 0 {
		opts.CommandTimeout = over.CommandTimeout
	}
	opts.Resume = over.Resume
	opts.Force, opts.Yes, opts.JSON, opts.Verbose = over.Force, over.Yes, over.JSON, over.Verbose

	// Mutual exclusion applies to the flags only. A configured `branch` plus an
	// explicit `--revision` is the normal CI invocation: the branch is the ref
	// the target fetches, the revision is the exact commit to check out.
	if err := validateSourceSelector(over); err != nil {
		return Options{}, err
	}
	if opts.Branch == "" && opts.Tag == "" && opts.Revision == "" {
		return Options{}, fmt.Errorf("remote %q has no branch configured and no --branch, --revision or --tag was given", name)
	}
	if !isKnownPublishStrategy(opts.Publish) {
		return Options{}, fmt.Errorf("unsupported publish strategy %q; use %s, %s or %s", opts.Publish, PublishAuto, PublishSymlink, PublishInPlace)
	}
	return opts, nil
}

// ResolveOptionsForTest exposes ResolveOptions to the tests/ package, which
// cannot call an unexported production helper across packages.
func ResolveOptionsForTest(cfg engine.Config, remote string, over Overrides) (Options, error) {
	return ResolveOptions(cfg, remote, over)
}

func isKnownPublishStrategy(value string) bool {
	switch value {
	case PublishAuto, PublishSymlink, PublishInPlace:
		return true
	default:
		return false
	}
}

func validateSourceSelector(over Overrides) error {
	chosen := 0
	for _, value := range []string{over.Branch, over.Revision, over.Tag} {
		if strings.TrimSpace(value) != "" {
			chosen++
		}
	}
	if chosen > 1 {
		return fmt.Errorf("--branch, --revision and --tag are mutually exclusive")
	}
	return nil
}

// errUnknownRemote names the remotes that do exist, so a typo is fixable from
// the error alone.
func errUnknownRemote(name string, cfg engine.Config) error {
	return fmt.Errorf("unknown remote %q; configured remotes: %s", name, configuredRemotes(cfg))
}

func configuredRemotes(cfg engine.Config) string {
	names := make([]string, 0, len(cfg.Remotes))
	for name := range cfg.Remotes {
		names = append(names, name)
	}
	engine.SortRemoteNames(names)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// mergeDeployConfig overlays a per-remote override block on the project block.
// Scalars win when the override sets a non-zero value; Settings merge key by
// key; Hooks replace wholesale, matching the documented list semantics of
// engine.MergeMap.
func mergeDeployConfig(project, override engine.DeployConfig) engine.DeployConfig {
	merged := project

	if override.KeepReleases > 0 {
		merged.KeepReleases = override.KeepReleases
	}
	if override.CommandTimeout != "" {
		merged.CommandTimeout = override.CommandTimeout
	}
	if override.ArtifactDir != "" {
		merged.ArtifactDir = override.ArtifactDir
	}
	if override.DBBackup {
		merged.DBBackup = true
	}
	if override.Verify.URL != "" {
		merged.Verify.URL = override.Verify.URL
	}
	if override.Verify.Timeout != "" {
		merged.Verify.Timeout = override.Verify.Timeout
	}
	if len(override.Hooks) > 0 {
		merged.Hooks = override.Hooks
	}
	merged.Settings = make(map[string]any, len(project.Settings)+len(override.Settings))
	for key, value := range project.Settings {
		merged.Settings[key] = value
	}
	for key, value := range override.Settings {
		merged.Settings[key] = value
	}
	return merged
}

func mergedSettings(settings map[string]any) map[string]any {
	merged := make(map[string]any, len(settings))
	for key, value := range settings {
		merged[key] = value
	}
	return merged
}
