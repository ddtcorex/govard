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
	NoiseExcludes []string                 `json:"noise_excludes"`
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
	normalized := strings.ToLower(strings.TrimSpace(framework))
	switch normalized {
	case "magento":
		return "magento2"
	case "wp":
		return "wordpress"
	default:
		return normalized
	}
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
		// Fallback to magento2 standard if framework not recognized
		config = frameworkManifest.Frameworks["magento2"]
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
