package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"govard/internal/conventions"

	"gopkg.in/yaml.v3"
)

type MigrationResult struct {
	ProjectName     string
	Framework       string
	PHPVersion      string
	NodeVersion     string
	ComposerVersion string
	DB              string
	DBVersion       string
	SearchService   string
	SearchVersion   string
	CacheService    string
	CacheVersion    string
	QueueService    string
	QueueVersion    string
	VarnishEnabled  bool
	VarnishVersion  string
	TablePrefix     string
	WebRoot         string
	Remotes         RemoteConfigMap

	// ExtraDomains are the hostnames the source tool served besides the primary
	// one; Govard keeps them as extra domains.
	ExtraDomains []string
	// WebServer is the front end the source tool used, when it named one.
	WebServer string
	// XdebugEnabled reports whether the source had debugging switched on.
	XdebugEnabled bool
	// VarnishSpecified reports whether the source said anything about Varnish. A
	// source that stays silent must leave the project's profile default alone.
	VarnishSpecified bool
	// Sources lists the configuration files that were read, in the order they
	// were applied. A layered DDEV project can answer from several files.
	Sources []string
	// Unmapped names every source key that has no Govard equivalent, with the
	// reason. Nothing is dropped silently.
	Unmapped []string
}

// ddevConfig is the union of the keys this importer understands. Anything it
// does not read is reported by ddevUnmappedKeys below.
type ddevConfig struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	PHPVersion string `yaml:"php_version"`
	WebServer  string `yaml:"webserver_type"`
	NodeJS     string `yaml:"nodejs_version"`
	Composer   string `yaml:"composer_version"`
	Database   struct {
		Type    string `yaml:"type"`
		Version string `yaml:"version"`
	} `yaml:"database"`
	AdditionalHostnames []string `yaml:"additional_hostnames"`
	AdditionalFQDNs     []string `yaml:"additional_fqdns"`
	XdebugEnabled       bool     `yaml:"xdebug_enabled"`

	UploadDirs            []string `yaml:"upload_dirs"`
	WebImageExtraPackages []string `yaml:"webimage_extra_packages"`
	OmitContainers        []string `yaml:"omit_containers"`
	Hooks                 any      `yaml:"hooks"`
	PerformanceMode       string   `yaml:"performance_mode"`
	MutagenEnabled        bool     `yaml:"mutagen_enabled"`
	WebExtraExposedPorts  any      `yaml:"web_extra_exposed_ports"`
	DatabaseSnapshot      bool     `yaml:"database_snapshot"`
	Timeout               string   `yaml:"timeout"`
	DisableSettingsMgmt   bool     `yaml:"disable_settings_management"`
	RouterHTTPSPort       string   `yaml:"router_https_port"`
	RouterHTTPPort        string   `yaml:"router_http_port"`
	ComposerRoot          string   `yaml:"composer_root"`
}

// MigrateFromDDEV reads .ddev/config.yaml followed by every .ddev/config.*.yaml
// in lexical order, later files winning, the way DDEV itself layers them. A key
// it cannot express is reported instead of being dropped.
func MigrateFromDDEV(root string) (MigrationResult, error) {
	dir := filepath.Join(root, ".ddev")
	base := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(base); err != nil {
		return MigrationResult{}, err
	}

	paths := []string{base}
	overrides, err := filepath.Glob(filepath.Join(dir, "config.*.yaml"))
	if err != nil {
		return MigrationResult{}, err
	}
	sort.Strings(overrides)
	paths = append(paths, overrides...)

	merged := ddevConfig{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return MigrationResult{}, err
		}
		if err := yaml.Unmarshal(data, &merged); err != nil {
			return MigrationResult{}, err
		}
	}

	result := MigrationResult{
		ProjectName:     merged.Name,
		Framework:       mapDDEVTypeToFramework(merged.Type),
		PHPVersion:      merged.PHPVersion,
		NodeVersion:     merged.NodeJS,
		ComposerVersion: merged.Composer,
		DB:              merged.Database.Type,
		DBVersion:       merged.Database.Version,
		WebServer:       mapDDEVWebServer(merged.WebServer),
		XdebugEnabled:   merged.XdebugEnabled,
		Sources:         paths,
	}

	// DDEV's additional_hostnames are short names under the project domain, so
	// they expand to <host>.<project>.test; additional_fqdns are already whole.
	for _, host := range merged.AdditionalHostnames {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if strings.Contains(host, ".") {
			result.ExtraDomains = append(result.ExtraDomains, host)
			continue
		}
		result.ExtraDomains = append(result.ExtraDomains, fmt.Sprintf("%s.%s.test", host, merged.Name))
	}
	result.ExtraDomains = append(result.ExtraDomains, merged.AdditionalFQDNs...)

	result.Unmapped = ddevUnmappedKeys(merged)

	return result, nil
}

// mapDDEVWebServer reduces a DDEV webserver_type to the web server Govard runs.
func mapDDEVWebServer(raw string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "apache"):
		return "apache"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "nginx"):
		return "nginx"
	default:
		return ""
	}
}

// ddevUnmappedKeys reports every key this importer saw but cannot express, so a
// migration summary can name them instead of pretending the project moved whole.
func ddevUnmappedKeys(cfg ddevConfig) []string {
	var unmapped []string
	add := func(present bool, key, reason string) {
		if present {
			unmapped = append(unmapped, key+": "+reason)
		}
	}

	add(len(cfg.UploadDirs) > 0, "upload_dirs", "Govard syncs media through remotes and snapshots, not upload directories")
	add(len(cfg.WebImageExtraPackages) > 0, "webimage_extra_packages", "Govard images are fixed per PHP version; add the extension to a blueprint instead")
	add(len(cfg.OmitContainers) > 0, "omit_containers", "Govard decides the service set from the profile, not from an omit list")
	add(cfg.Hooks != nil, "hooks", "Govard hooks are declared in .govard.yml; re-add them there")
	add(strings.TrimSpace(cfg.PerformanceMode) != "", "performance_mode", "Govard mounts the project directly on Linux")
	add(cfg.MutagenEnabled, "mutagen_enabled", "Govard does not use Mutagen")
	add(cfg.WebExtraExposedPorts != nil, "web_extra_exposed_ports", "expose the port through the proxy config instead")
	add(cfg.DatabaseSnapshot, "database_snapshot", "use govard snapshot for local database checkpoints")
	add(strings.TrimSpace(cfg.Timeout) != "", "timeout", "Govard waits per command, not per project")
	add(cfg.DisableSettingsMgmt, "disable_settings_management", "Govard always manages .govard.yml and the container settings it renders")
	add(strings.TrimSpace(cfg.RouterHTTPPort) != "" || strings.TrimSpace(cfg.RouterHTTPSPort) != "", "router_http_port/router_https_port", "Govard serves through its shared proxy on 80 and 443")
	add(strings.TrimSpace(cfg.ComposerRoot) != "", "composer_root", "Govard expects the application at the project root")

	return unmapped
}

func MigrateFromWarden(root string) (MigrationResult, error) {
	env := ParseDotEnv(filepath.Join(root, ".env"))

	wardenConfigPath := filepath.Join(root, ".warden", "warden-env.yml")
	var warden struct {
		WardenEnvName string `yaml:"warden_env_name"`
		WardenEnvType string `yaml:"warden_env_type"`
	}
	if data, err := os.ReadFile(wardenConfigPath); err == nil {
		_ = yaml.Unmarshal(data, &warden)
	}

	result := MigrationResult{
		ProjectName:     env["WARDEN_ENV_NAME"],
		Framework:       mapWardenTypeToFramework(env["WARDEN_ENV_TYPE"]),
		PHPVersion:      env[conventions.EnvPHPVersion],
		NodeVersion:     env["NODE_VERSION"],
		ComposerVersion: env["COMPOSER_VERSION"],
		DBVersion:       env["MYSQL_DISTRIBUTION_VERSION"],
		VarnishVersion:  env["VARNISH_VERSION"],
		WebRoot:         env["WARDEN_WEB_ROOT"],
		Remotes:         make(RemoteConfigMap),
	}

	if result.ProjectName == "" {
		result.ProjectName = warden.WardenEnvName
	}
	if result.Framework == "" {
		result.Framework = mapWardenTypeToFramework(warden.WardenEnvType)
	}
	if FrameworkSupportsTablePrefix(result.Framework) {
		result.TablePrefix = NormalizeTablePrefix(env["WARDEN_TABLE_PREFIX"])
	}

	if env["MYSQL_DISTRIBUTION"] != "" {
		result.DB = strings.ToLower(env["MYSQL_DISTRIBUTION"])
	}
	if env["WARDEN_DB"] == "0" {
		result.DB = "none"
	}

	if env["WARDEN_REDIS"] == "1" {
		result.CacheService = "redis"
		result.CacheVersion = env["REDIS_VERSION"]
	}
	if env["WARDEN_RABBITMQ"] == "1" {
		result.QueueService = "rabbitmq"
		result.QueueVersion = env["RABBITMQ_VERSION"]
	}
	if env["WARDEN_VARNISH"] == "1" {
		result.VarnishEnabled = true
	}

	if env["WARDEN_OPENSEARCH"] == "1" {
		result.SearchService = "opensearch"
		result.SearchVersion = env["OPENSEARCH_VERSION"]
	} else if env["WARDEN_ELASTICSEARCH"] == "1" {
		result.SearchService = "elasticsearch"
		result.SearchVersion = env[conventions.EnvElasticsearchVersion]
	}

	// Legacy Warden SSH variables
	if host := env["WARDEN_SSH_HOST"]; host != "" {
		result.Remotes["production"] = RemoteConfig{
			Host:      host,
			User:      env["WARDEN_SSH_USER"],
			Path:      env["WARDEN_SSH_PATH"],
			Protected: BoolPtr(true),
		}
	}

	// Modern Warden Custom Commands remote variables
	// Format: REMOTE_{ENV}_{PROPERTY} where PROPERTY is HOST, USER, PORT, PATH, URL
	remotes := make(map[string]*RemoteConfig)
	for key, value := range env {
		if !strings.HasPrefix(key, "REMOTE_") {
			continue
		}
		parts := strings.Split(key, "_")
		if len(parts) < 3 {
			continue
		}

		envName := strings.ToLower(parts[1])
		switch envName {
		case "prod":
			envName = "production"
		case "staging":
			envName = "staging"
		case "dev":
			envName = "development"
		default:
			// Custom environment names (e.g. REMOTE_QA_HOST) pass through as-is (lowercase).
		}

		property := parts[len(parts)-1]
		if _, ok := remotes[envName]; !ok {
			remotes[envName] = &RemoteConfig{
				Port: 22,
			}
		}

		switch property {
		case "HOST":
			remotes[envName].Host = value
		case "USER":
			remotes[envName].User = value
		case "PORT":
			if port, err := strconv.Atoi(value); err == nil {
				remotes[envName].Port = port
			}
		case "PATH":
			remotes[envName].Path = value
		case "URL":
			remotes[envName].URL = value
		}
	}

	for name, cfg := range remotes {
		if cfg.Host != "" {
			result.Remotes[name] = *cfg
		}
	}

	return result, nil
}

func mapDDEVTypeToFramework(ddevType string) string {
	return lookupMigrationFramework("ddev", ddevType)
}

func mapWardenTypeToFramework(wardenType string) string {
	return lookupMigrationFramework("warden", wardenType)
}

func ParseDotEnv(path string) map[string]string {
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
