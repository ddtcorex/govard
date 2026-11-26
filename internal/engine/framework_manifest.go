package engine

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed framework_manifest.json
var frameworkManifestJSON []byte

const (
	FrameworkMediaModeAll       = "all"
	FrameworkMediaModeOptimized = "optimized"
	FrameworkMediaModeMinimal   = "minimal"
)

type FrameworkTablesManifest struct {
	Shared     FrameworkSharedConfig              `json:"_shared"`
	Frameworks map[string]FrameworkManifestConfig `json:"frameworks"`
}

type FrameworkSharedConfig struct {
	Sync FrameworkSharedSyncConfig `json:"sync"`
}

type FrameworkSharedSyncConfig struct {
	GlobalNoiseExcludes   []string `json:"global_noise_excludes"`
	SensitivePathExcludes []string `json:"sensitive_path_excludes"`
	FallbackNoiseExcludes []string `json:"fallback_noise_excludes"`
	MediaCommonExcludes   []string `json:"media_common_excludes"`
	MediaFallbackNonAll   []string `json:"media_fallback_non_all_excludes"`
	MediaMinimalExcludes  []string `json:"media_minimal_excludes"`
}

type FrameworkManifestConfig struct {
	Ignored   []string               `json:"ignored"`
	Sensitive []string               `json:"sensitive"`
	Paths     FrameworkPathConfig    `json:"paths"`
	Sync      FrameworkSyncConfig    `json:"sync"`
	Features  FrameworkFeatureConfig `json:"features"`
}

type FrameworkPathConfig struct {
	LocalMedia        string                      `json:"local_media"`
	RemoteMedia       string                      `json:"remote_media"`
	WebRootCandidates []FrameworkWebRootCandidate `json:"web_root_candidates"`
}

type FrameworkWebRootCandidate struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

type FrameworkSyncConfig struct {
	NoiseExcludes []string `json:"noise_excludes"`
	// CloneExcludes lists the paths a bootstrap file clone must never
	// overwrite or pull from the remote beyond the generic ones (VCS, .env,
	// editor and dependency directories): local-only config, generated code
	// and runtime state the framework owns. The media directory is excluded
	// separately from Paths.LocalMedia because media has its own sync.
	CloneExcludes []string                 `json:"clone_excludes,omitempty"`
	MediaExcludes FrameworkMediaExcludeSet `json:"media_excludes"`
}

type FrameworkMediaExcludeSet struct {
	NonAll    []string `json:"non_all"`
	Optimized []string `json:"optimized"`
	Minimal   []string `json:"minimal"`
}

type FrameworkFeatureConfig struct {
	RequiresRunningEnvForFreshInstall bool `json:"requires_running_env_for_fresh_install"`
	SupportsPostClone                 bool `json:"supports_post_clone"`
}

var (
	frameworkManifest     FrameworkTablesManifest
	frameworkManifestOnce sync.Once
)

func loadFrameworkTablesManifest() {
	frameworkManifestOnce.Do(func() {
		_ = json.Unmarshal(frameworkManifestJSON, &frameworkManifest)
		if frameworkManifest.Frameworks == nil {
			frameworkManifest.Frameworks = make(map[string]FrameworkManifestConfig)
		}
	})
}

func normalizeFrameworkManifestKey(framework string) string {
	return NormalizeFrameworkAlias(framework)
}

func getFrameworkManifestConfig(framework string) (FrameworkManifestConfig, bool) {
	loadFrameworkTablesManifest()
	config, ok := frameworkManifest.Frameworks[normalizeFrameworkManifestKey(framework)]
	return config, ok
}

// GetFrameworkManifestConfig returns the raw manifest configuration for
// framework (sync/media excludes, sensitive tables, feature flags), as
// loaded from framework_manifest.json.
func GetFrameworkManifestConfig(framework string) (FrameworkManifestConfig, bool) {
	return getFrameworkManifestConfig(framework)
}

// RegisterFrameworkManifest registers manifest as the FrameworkManifestConfig
// for framework, keyed the same way GetFrameworkManifestConfig looks it
// up. Called from frameworks.Register (alongside RegisterDetection and
// RegisterFrameworkConfig) so a framework package can own its own
// FrameworkManifestConfig literal instead of an entry in
// framework_manifest.json's "frameworks" object. The "_shared" block
// (global noise/media excludes) stays in framework_manifest.json - it's
// genuinely cross-framework data, not per-framework. Not safe for
// concurrent calls; intended usage is registration during package
// init(), before GetFrameworkManifestConfig is ever called.
func RegisterFrameworkManifest(framework string, manifest FrameworkManifestConfig) {
	loadFrameworkTablesManifest()
	frameworkManifest.Frameworks[normalizeFrameworkManifestKey(framework)] = manifest
}

func appendStrings(dst []string, values []string) []string {
	if len(values) == 0 {
		return dst
	}
	return append(dst, values...)
}

// GetFrameworkIgnoredTables returns the list of tables to ignore for a given framework
// based on whether noise (logs/cache) or PII (sensitive data) filters are active.
func GetFrameworkIgnoredTables(framework string, noNoise bool, noPII bool) []string {
	if !noNoise && !noPII {
		return nil
	}

	config, ok := getFrameworkManifestConfig(framework)
	if !ok {
		return nil
	}

	tables := make([]string, 0)
	if noNoise {
		tables = append(tables, config.Ignored...)
	}
	if noPII {
		tables = append(tables, config.Sensitive...)
	}
	return tables
}

func ResolveFrameworkLocalMediaSubpath(framework string) string {
	config, ok := getFrameworkManifestConfig(framework)
	if ok && strings.TrimSpace(config.Paths.LocalMedia) != "" {
		return strings.TrimSpace(config.Paths.LocalMedia)
	}
	return "public/media"
}

func ResolveFrameworkRemoteMediaSubpath(framework string) string {
	config, ok := getFrameworkManifestConfig(framework)
	if ok && strings.TrimSpace(config.Paths.RemoteMedia) != "" {
		return strings.TrimSpace(config.Paths.RemoteMedia)
	}
	return "public/media"
}

func DetectFrameworkWebRoot(root string, framework string) string {
	config, ok := getFrameworkManifestConfig(framework)
	if !ok {
		return ""
	}

	for _, candidate := range config.Paths.WebRootCandidates {
		relativePath := strings.TrimSpace(candidate.Path)
		value := strings.TrimSpace(candidate.Value)
		if relativePath == "" || value == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relativePath))); err == nil {
			return value
		}
	}

	return ""
}

func GetFrameworkSyncNoiseExcludes(framework string) []string {
	loadFrameworkTablesManifest()

	excludes := make([]string, 0)
	excludes = appendStrings(excludes, frameworkManifest.Shared.Sync.GlobalNoiseExcludes)
	excludes = appendStrings(excludes, frameworkManifest.Shared.Sync.SensitivePathExcludes)

	config, ok := getFrameworkManifestConfig(framework)
	if ok && config.Sync.NoiseExcludes != nil {
		return appendStrings(excludes, config.Sync.NoiseExcludes)
	}

	return appendStrings(excludes, frameworkManifest.Shared.Sync.FallbackNoiseExcludes)
}

// GetSyncProtectedExcludes returns the paths a destructive sync must never
// remove from either endpoint: source control, local secrets, installed
// dependencies, and the paths the framework itself declares as local only
// (its clone excludes: env.php and its crypt key for Magento, wp-config.php for
// WordPress). None of them can be regenerated from a deploy release.
//
// Unlike GetFrameworkSyncNoiseExcludes it carries none of the framework noise
// filters -- those are what `--no-noise` is for, and they stay opt in.
var govardOwnedSyncPaths = []string{".govard*.yml", ".govard/"}

func GetSyncProtectedExcludes(framework string) []string {
	loadFrameworkTablesManifest()

	shared := frameworkManifest.Shared.Sync
	patterns := append([]string{}, shared.GlobalNoiseExcludes...)
	patterns = append(patterns, shared.SensitivePathExcludes...)
	// Govard's own project files belong to no framework and are not framework
	// data, so they are listed here rather than in the manifest. A deploy release
	// never carries them (the config is ignored on most machines, the state
	// directory always is), which makes them deletion candidates on every pull.
	patterns = append(patterns, govardOwnedSyncPaths...)
	if config, ok := getFrameworkManifestConfig(framework); ok {
		patterns = append(patterns, config.Sync.CloneExcludes...)
	}

	excludes := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		excludes = append(excludes, trimmed)
	}
	return excludes
}

func GetFrameworkMediaExcludes(framework string, mode string) []string {
	loadFrameworkTablesManifest()

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" || normalizedMode == FrameworkMediaModeAll {
		return nil
	}

	excludes := make([]string, 0)
	excludes = appendStrings(excludes, frameworkManifest.Shared.Sync.MediaCommonExcludes)

	config, ok := getFrameworkManifestConfig(framework)
	if ok && config.Sync.MediaExcludes.NonAll != nil {
		excludes = appendStrings(excludes, config.Sync.MediaExcludes.NonAll)
	} else {
		excludes = appendStrings(excludes, frameworkManifest.Shared.Sync.MediaFallbackNonAll)
	}

	switch normalizedMode {
	case FrameworkMediaModeOptimized:
		if ok {
			excludes = appendStrings(excludes, config.Sync.MediaExcludes.Optimized)
		}
	case FrameworkMediaModeMinimal:
		if ok {
			excludes = appendStrings(excludes, config.Sync.MediaExcludes.Minimal)
		}
		excludes = appendStrings(excludes, frameworkManifest.Shared.Sync.MediaMinimalExcludes)
	}

	return excludes
}

func FrameworkRequiresRunningEnvForFreshInstall(framework string) bool {
	config, ok := getFrameworkManifestConfig(framework)
	if !ok {
		return true
	}
	return config.Features.RequiresRunningEnvForFreshInstall
}

func FrameworkSupportsPostClone(framework string) bool {
	config, ok := getFrameworkManifestConfig(framework)
	if !ok {
		return false
	}
	return config.Features.SupportsPostClone
}
