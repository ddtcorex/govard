package engine

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type MigrationResult struct {
	ProjectName string
	Recipe      string
	PHPVersion  string
	DBType      string
	DBVersion   string
	WebRoot     string
	Remotes     map[string]RemoteConfig
}

func MigrateFromDDEV(root string) (MigrationResult, error) {
	configPath := filepath.Join(root, ".ddev", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return MigrationResult{}, err
	}

	var ddev struct {
		Name       string `yaml:"name"`
		Type       string `yaml:"type"`
		PHPVersion string `yaml:"php_version"`
		Database   struct {
			Type    string `yaml:"type"`
			Version string `yaml:"version"`
		} `yaml:"database"`
	}

	if err := yaml.Unmarshal(data, &ddev); err != nil {
		return MigrationResult{}, err
	}

	result := MigrationResult{
		ProjectName: ddev.Name,
		Recipe:      mapDDEVTypeToRecipe(ddev.Type),
		PHPVersion:  ddev.PHPVersion,
		DBType:      ddev.Database.Type,
		DBVersion:   ddev.Database.Version,
	}

	return result, nil
}

func MigrateFromWarden(root string) (MigrationResult, error) {
	env := parseDotEnv(filepath.Join(root, ".env"))

	wardenConfigPath := filepath.Join(root, ".warden", "warden-env.yml")
	var warden struct {
		WardenEnvName string `yaml:"warden_env_name"`
		WardenEnvType string `yaml:"warden_env_type"`
	}
	if data, err := os.ReadFile(wardenConfigPath); err == nil {
		_ = yaml.Unmarshal(data, &warden)
	}

	result := MigrationResult{
		ProjectName: env["WARDEN_ENV_NAME"],
		Recipe:      mapWardenTypeToRecipe(env["WARDEN_ENV_TYPE"]),
		WebRoot:     env["WARDEN_WEB_ROOT"],
		Remotes:     make(map[string]RemoteConfig),
	}

	if result.ProjectName == "" {
		result.ProjectName = warden.WardenEnvName
	}
	if result.Recipe == "" {
		result.Recipe = mapWardenTypeToRecipe(warden.WardenEnvType)
	}

	// Try to extract remote info from .env
	if host := env["WARDEN_SSH_HOST"]; host != "" {
		remote := RemoteConfig{
			Host:        host,
			User:        env["WARDEN_SSH_USER"],
			Path:        env["WARDEN_SSH_PATH"],
			Environment: "production",
			Capabilities: RemoteCapabilities{
				Files:  true,
				Media:  true,
				DB:     true,
				Deploy: false,
			},
		}
		result.Remotes["production"] = remote
	}

	return result, nil
}

func mapDDEVTypeToRecipe(ddevType string) string {
	switch ddevType {
	case "magento2":
		return "magento2"
	case "magento":
		return "magento1"
	case "laravel":
		return "laravel"
	case "drupal7", "drupal8", "drupal9", "drupal10", "drupal11":
		return "drupal"
	case "symfony":
		return "symfony"
	case "shopware6":
		return "shopware"
	case "wordpress":
		return "wordpress"
	default:
		return ddevType
	}
}

func mapWardenTypeToRecipe(wardenType string) string {
	switch wardenType {
	case "magento2":
		return "magento2"
	case "magento1":
		return "magento1"
	case "laravel":
		return "laravel"
	case "symfony":
		return "symfony"
	case "shopware":
		return "shopware"
	case "wordpress":
		return "wordpress"
	default:
		return wardenType
	}
}

func parseDotEnv(path string) map[string]string {
	env := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return env
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes if present
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			env[key] = value
		}
	}
	return env
}
