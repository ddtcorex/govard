package engine

import "strings"

// DefaultKeepReleases is how many releases a deploy keeps on the target. It
// mirrors the reference project's Deployer setting so a migrated project does
// not silently change retention.
const DefaultKeepReleases = 5

// DeployConfig is the project-level `deploy:` block. It carries only
// framework-neutral settings; anything a framework recipe needs travels through
// Settings untouched, because the core never interprets it.
type DeployConfig struct {
	KeepReleases   int                `yaml:"keep_releases,omitempty"`
	CommandTimeout string             `yaml:"command_timeout,omitempty"`
	LockStaleAfter string             `yaml:"lock_stale_after,omitempty"`
	ArtifactDir    string             `yaml:"artifact_dir,omitempty"`
	DBBackup       bool               `yaml:"db_backup,omitempty"`
	Verify         DeployVerifyConfig `yaml:"verify,omitempty"`
	Settings       map[string]any     `yaml:"settings,omitempty"`
	Hooks          []DeployHookConfig `yaml:"hooks,omitempty"`
}

// DeployVerifyConfig configures the post-publish HTTP check.
type DeployVerifyConfig struct {
	URL     string `yaml:"url,omitempty"`
	Timeout string `yaml:"timeout,omitempty"`
}

// DeployHookConfig is one project hook. It is deliberately not HookStep: a
// deploy hook carries an anchor, a position and an ordering key, because hooks
// attach to any task, stage or other hook.
type DeployHookConfig struct {
	Name     string `yaml:"name"`
	On       string `yaml:"on"`
	Position string `yaml:"position,omitempty"`
	Order    int    `yaml:"order,omitempty"`
	Run      string `yaml:"run"`
	RunOn    string `yaml:"run_on,omitempty"`
	Optional bool   `yaml:"optional,omitempty"`
}

// KeepReleasesOr returns the configured keep_releases or the default. A
// non-positive value means "not configured", which is also what an omitted YAML
// key decodes to.
func (c DeployConfig) KeepReleasesOr() int {
	if c.KeepReleases <= 0 {
		return DefaultKeepReleases
	}
	return c.KeepReleases
}

// NormalizeDeployConfig trims deploy configuration and lowercases the values
// that are matched against a closed set. Durations stay strings: an invalid
// duration must be reported by the command layer with an actionable message,
// not silently dropped during normalization.
func NormalizeDeployConfig(config *Config) {
	if config == nil {
		return
	}

	config.Deploy.ArtifactDir = strings.TrimSpace(config.Deploy.ArtifactDir)
	config.Deploy.CommandTimeout = strings.TrimSpace(config.Deploy.CommandTimeout)
	config.Deploy.LockStaleAfter = strings.TrimSpace(config.Deploy.LockStaleAfter)
	config.Deploy.Verify.URL = strings.TrimSpace(config.Deploy.Verify.URL)
	config.Deploy.Verify.Timeout = strings.TrimSpace(config.Deploy.Verify.Timeout)

	for idx := range config.Deploy.Hooks {
		hook := &config.Deploy.Hooks[idx]
		hook.Name = strings.TrimSpace(hook.Name)
		hook.On = strings.TrimSpace(hook.On)
		hook.Position = strings.ToLower(strings.TrimSpace(hook.Position))
		hook.RunOn = strings.ToLower(strings.TrimSpace(hook.RunOn))
	}

	for name, remote := range config.Remotes {
		remote.Branch = strings.TrimSpace(remote.Branch)
		remote.Repository = strings.TrimSpace(remote.Repository)
		remote.DeployPath = strings.TrimSpace(remote.DeployPath)
		remote.Publish = strings.ToLower(strings.TrimSpace(remote.Publish))

		if remote.Deploy != nil {
			override := *remote.Deploy
			override.ArtifactDir = strings.TrimSpace(override.ArtifactDir)
			override.CommandTimeout = strings.TrimSpace(override.CommandTimeout)
			override.LockStaleAfter = strings.TrimSpace(override.LockStaleAfter)
			override.Verify.URL = strings.TrimSpace(override.Verify.URL)
			override.Verify.Timeout = strings.TrimSpace(override.Verify.Timeout)
			for idx := range override.Hooks {
				hook := &override.Hooks[idx]
				hook.Name = strings.TrimSpace(hook.Name)
				hook.On = strings.TrimSpace(hook.On)
				hook.Position = strings.ToLower(strings.TrimSpace(hook.Position))
				hook.RunOn = strings.ToLower(strings.TrimSpace(hook.RunOn))
			}
			remote.Deploy = &override
		}

		config.Remotes[name] = remote
	}
}
