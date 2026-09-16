package engine

import (
	"errors"
	"fmt"
	"strings"
)

// DefaultKeepReleases is how many releases a deploy keeps on the target. It
// mirrors the reference project's Deployer setting so a migrated project does
// not silently change retention.
const DefaultKeepReleases = 5

// DeployConfig is the project-level `deploy:` block. It carries only
// framework-neutral settings; anything a framework recipe needs travels through
// Settings untouched, because the core never interprets it.
type DeployConfig struct {
	KeepReleases   int    `yaml:"keep_releases,omitempty"`
	CommandTimeout string `yaml:"command_timeout,omitempty"`
	LockStaleAfter string `yaml:"lock_stale_after,omitempty"`
	// MaintenanceTimeout bounds one step inside the maintenance window.
	MaintenanceTimeout string `yaml:"maintenance_timeout,omitempty"`
	ArtifactDir        string `yaml:"artifact_dir,omitempty"`
	DBBackup           bool   `yaml:"db_backup,omitempty"`
	// Deploy topology defaults. Each is the project-wide default for the
	// same-named key under `remotes.<name>.deploy:`; an explicitly configured
	// remote value always wins. There is exactly one place per layer, so no
	// conflict check is needed.
	Repository string             `yaml:"repository,omitempty"`
	Branch     string             `yaml:"branch,omitempty"`
	Publish    string             `yaml:"publish,omitempty"`
	DeployPath string             `yaml:"deploy_path,omitempty"`
	Verify     DeployVerifyConfig `yaml:"verify,omitempty"`
	Settings   map[string]any     `yaml:"settings,omitempty"`
	Hooks      []DeployHookConfig `yaml:"hooks,omitempty"`
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

// RemovedRemoteDeployKeys are the former remote-level topology shorthands.
// They were removed in favor of the single nested form
// `remotes.<name>.deploy.<key>`: two spellings for one value caused the
// conflict class the old checker patched over. The YAML decoder drops unknown
// keys silently, so the loader must reject them explicitly on the raw merged
// map before decoding, or a deploy would run against the wrong ref with no
// error at all.
var RemovedRemoteDeployKeys = []string{"branch", "repository", "publish", "deploy_path"}

// ErrRemovedRemoteDeployKey marks a config that still uses a removed flat
// remote topology key. It travels wrapped so command layers can classify the
// failure as a configuration error (exit 4) instead of a usage error.
var ErrRemovedRemoteDeployKey = errors.New("flat remote deploy topology is no longer supported")

// RejectRemovedRemoteDeployKeys fails a merged config whose remotes section
// still uses a removed flat topology key. An explicitly empty value reads the
// same as an absent one, so only a set value is rejected.
func RejectRemovedRemoteDeployKeys(merged map[string]interface{}) error {
	remotes, ok := stringMap(merged["remotes"])
	if !ok {
		return nil
	}
	for name, raw := range remotes {
		remote, ok := stringMap(raw)
		if !ok {
			continue
		}
		for _, key := range RemovedRemoteDeployKeys {
			value, present := remote[key]
			if !present || value == nil {
				continue
			}
			if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
				continue
			}
			return fmt.Errorf("%w: remotes.%s sets %q; move it under remotes.%s.deploy.%s", ErrRemovedRemoteDeployKey, name, key, name, key)
		}
	}
	return nil
}

// stringMap reads a decoded YAML mapping regardless of the key type the
// decoder produced.
func stringMap(raw interface{}) (map[string]interface{}, bool) {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return typed, true
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[text] = value
		}
		return converted, true
	default:
		return nil, false
	}
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
	config.Deploy.MaintenanceTimeout = strings.TrimSpace(config.Deploy.MaintenanceTimeout)
	config.Deploy.Repository = strings.TrimSpace(config.Deploy.Repository)
	config.Deploy.Branch = strings.TrimSpace(config.Deploy.Branch)
	config.Deploy.Publish = strings.ToLower(strings.TrimSpace(config.Deploy.Publish))
	config.Deploy.DeployPath = strings.TrimSpace(config.Deploy.DeployPath)
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
		if remote.Deploy != nil {
			override := *remote.Deploy
			override.ArtifactDir = strings.TrimSpace(override.ArtifactDir)
			override.CommandTimeout = strings.TrimSpace(override.CommandTimeout)
			override.LockStaleAfter = strings.TrimSpace(override.LockStaleAfter)
			override.MaintenanceTimeout = strings.TrimSpace(override.MaintenanceTimeout)
			override.Repository = strings.TrimSpace(override.Repository)
			override.Branch = strings.TrimSpace(override.Branch)
			override.Publish = strings.ToLower(strings.TrimSpace(override.Publish))
			override.DeployPath = strings.TrimSpace(override.DeployPath)
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
