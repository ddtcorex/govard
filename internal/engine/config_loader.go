package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	BaseConfigFile  = "govard.yml"
	LocalConfigFile = "govard.local.yml"
)

var validEnvNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func ResolveConfigLayerPaths(root string) []string {
	paths := []string{
		filepath.Join(root, BaseConfigFile),
		filepath.Join(root, LocalConfigFile),
		filepath.Join(root, ProjectLocalConfigPath),
	}

	envName := strings.TrimSpace(os.Getenv("GOVARD_ENV"))
	if envName == "" {
		return paths
	}

	if !validEnvNamePattern.MatchString(envName) {
		return paths
	}

	paths = append(paths, filepath.Join(root, fmt.Sprintf("govard.%s.yml", envName)))
	paths = append(paths, filepath.Join(root, ProjectExtensionsDir, fmt.Sprintf("govard.%s.yml", envName)))
	return paths
}

func LoadConfigFromDir(root string, requireBase bool) (Config, []string, error) {
	paths := ResolveConfigLayerPaths(root)
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

		mergeMap(merged, layer)
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
		cfg.Domain = cfg.ProjectName + ".test"
	}

	NormalizeConfig(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return Config{}, nil, err
	}

	return cfg, loaded, nil
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
				cfg.Domain = cfg.ProjectName + ".test"
			}
			NormalizeConfig(&cfg)
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
		cfg.Domain = cfg.ProjectName + ".test"
	}

	NormalizeConfig(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func inferProjectName(root string) string {
	base := strings.TrimSpace(filepath.Base(root))
	base = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	return base
}

func mergeMap(dst, src map[string]interface{}) {
	for key, val := range src {
		srcMap, srcIsMap := val.(map[string]interface{})
		dstMap, dstIsMap := dst[key].(map[string]interface{})

		if srcIsMap && dstIsMap {
			mergeMap(dstMap, srcMap)
			dst[key] = dstMap
			continue
		}

		dst[key] = val
	}
}
