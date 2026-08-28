package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"govard/internal/conventions"
)

const (
	BaseConfigFile  = conventions.BaseConfigFile
	LocalConfigFile = conventions.LocalConfigFile
)

var validEnvNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func ResolveConfigLayerPaths(root string) []string {
	return ResolveConfigLayerPathsWithProfile(root, "")
}

// ResolveConfigLayerPathsWithProfile builds the ordered list of config files.
// Load order: Base → Profile → Local → ProjectLocal → GOVARD_ENV → ProjectEnv.
// Profile is inserted after base so local overrides always win; GOVARD_ENV layers are last so env-specific overrides win over local dev overrides.
func ResolveConfigLayerPathsWithProfile(root, profile string) []string {
	paths := []string{
		filepath.Join(root, BaseConfigFile),
	}

	// Profile layer (team-committed scope file)
	profile = strings.TrimSpace(profile)
	if profile != "" && validEnvNamePattern.MatchString(profile) {
		paths = append(paths, filepath.Join(root, fmt.Sprintf(".govard.%s.yml", profile)))
	}

	// Local overrides (personal, never committed)
	paths = append(paths, filepath.Join(root, LocalConfigFile))
	paths = append(paths, filepath.Join(root, ProjectLocalConfigPath))

	envName := strings.TrimSpace(os.Getenv("GOVARD_ENV"))
	if envName != "" && validEnvNamePattern.MatchString(envName) {
		paths = append(paths, filepath.Join(root, fmt.Sprintf(".govard.%s.yml", envName)))
		paths = append(paths, filepath.Join(root, ProjectExtensionsDir, fmt.Sprintf(".govard.%s.yml", envName)))
	}

	return paths
}

func LoadConfigFromDir(root string, requireBase bool) (Config, []string, error) {
	return LoadConfigFromDirWithProfile(root, requireBase, "")
}

// LoadConfigFromDirWithProfile loads config with an optional profile layer.
// Profile files (.govard.[profile].yml) are merged after base but before local.
func LoadConfigFromDirWithProfile(root string, requireBase bool, profile string) (Config, []string, error) {
	paths := ResolveConfigLayerPathsWithProfile(root, profile)
	merged := map[string]interface{}{}
	loaded := make([]string, 0, len(paths))

	for idx, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if idx == 0 && requireBase {
					return Config{}, nil, fmt.Errorf("%s not found", BaseConfigFile)
				}
				continue
			}
			return Config{}, nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		var layer map[string]interface{}
		if err := yaml.Unmarshal(data, &layer); err != nil {
			return Config{}, nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		if layer == nil {
			layer = map[string]interface{}{}
		}

		MergeMap(merged, layer)
		loaded = append(loaded, path)
	}

	if requireBase && len(loaded) == 0 {
		return Config{}, nil, fmt.Errorf("%s not found", BaseConfigFile)
	}

	var cfg Config
	payload, err := yaml.Marshal(merged)
	if err != nil {
		return Config{}, nil, fmt.Errorf("failed to marshal merged config: %w", err)
	}
	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return Config{}, nil, fmt.Errorf("failed to decode merged config: %w", err)
	}

	if cfg.ProjectName == "" {
		cfg.ProjectName = inferProjectName(root)
	}
	if cfg.Domain == "" && cfg.ProjectName != "" {
		cfg.Domain = inferProjectDomain(root)
	}

	cfg.Profile = profile
	NormalizeConfig(&cfg, root)
	if err := ValidateConfig(cfg); err != nil {
		return Config{}, nil, err
	}

	return cfg, loaded, nil
}

// LoadRawConfigFromDir loads the base configuration WITHOUT applying normalization or validation.
// This is used to detect which fields were explicitly provided in the file by the user.
func LoadRawConfigFromDir(root string, requireBase bool) (Config, error) {
	path := filepath.Join(root, BaseConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireBase {
				return Config{}, fmt.Errorf("%s not found", BaseConfigFile)
			}
			return Config{
				ProjectName: inferProjectName(root),
			}, nil
		}
		return Config{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return cfg, nil
}

// LoadRawConfigFromDirWithProfile loads the configuration WITHOUT applying normalization or validation,
// but merging all profile and local layers just like LoadConfigFromDirWithProfile.
// This is used to detect which fields were explicitly provided in the layered files by the user.
func LoadRawConfigFromDirWithProfile(root string, requireBase bool, profile string) (Config, error) {
	paths := ResolveConfigLayerPathsWithProfile(root, profile)
	merged := map[string]interface{}{}
	loaded := make([]string, 0, len(paths))

	for idx, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if idx == 0 && requireBase {
					return Config{}, fmt.Errorf("%s not found", BaseConfigFile)
				}
				continue
			}
			return Config{}, fmt.Errorf("failed to read %s: %w", path, err)
		}

		var layer map[string]interface{}
		if err := yaml.Unmarshal(data, &layer); err != nil {
			return Config{}, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		if layer == nil {
			layer = map[string]interface{}{}
		}

		MergeMap(merged, layer)
		loaded = append(loaded, path)
	}

	if requireBase && len(loaded) == 0 {
		return Config{}, fmt.Errorf("%s not found", BaseConfigFile)
	}

	var cfg Config
	payload, err := yaml.Marshal(merged)
	if err != nil {
		return Config{}, fmt.Errorf("failed to marshal merged config: %w", err)
	}
	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to decode merged config: %w", err)
	}

	if cfg.ProjectName == "" {
		cfg.ProjectName = inferProjectName(root)
	}

	cfg.Profile = profile
	return cfg, nil
}

func LoadBaseConfigFromDir(root string, requireBase bool) (Config, error) {
	path := filepath.Join(root, BaseConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireBase {
				return Config{}, fmt.Errorf("%s not found", BaseConfigFile)
			}
			cfg := Config{
				ProjectName: inferProjectName(root),
			}
			if cfg.ProjectName != "" {
				cfg.Domain = inferProjectDomain(root)
			}
			NormalizeConfig(&cfg, root)
			return cfg, nil
		}
		return Config{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if cfg.ProjectName == "" {
		cfg.ProjectName = inferProjectName(root)
	}
	if cfg.Domain == "" && cfg.ProjectName != "" {
		cfg.Domain = inferProjectDomain(root)
	}

	NormalizeConfig(&cfg, root)
	if err := ValidateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func inferProjectName(root string) string {
	base := strings.TrimSpace(filepath.Base(root))
	return NormalizeProjectName(base)
}

func inferProjectDomain(root string) string {
	base := strings.TrimSpace(filepath.Base(root))
	base = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	if strings.HasSuffix(base, ".test") {
		return base
	}

	name := NormalizeProjectName(base)
	if name == "" {
		return ""
	}
	return name + ".test"
}

func InferProjectDomain(root string) string {
	return inferProjectDomain(root)
}
