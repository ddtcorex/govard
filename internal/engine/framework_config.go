package engine

import (
	"govard/internal/conventions"
	"strings"
)

// FrameworkConfig defines the configuration for a specific framework
type FrameworkConfig struct {
	Name               string
	Runtime            string
	AppService         string
	AppWorkdir         string
	NGINXPUBLIC        string
	NGINXTemplate      string
	DatabaseName       string
	DefaultPHP         string
	DefaultPythonVer   string
	DefaultNodeVer     string
	DefaultDB          string
	DefaultDBVer       string
	DefaultMySQLVer    string
	DefaultNginxVer    string
	DefaultApacheVer   string
	DefaultCacheVer    string
	DefaultSearchVer   string
	DefaultVarnishVer  string
	DefaultQueueVer    string
	DefaultWebServer   string
	DefaultSearch      string
	DefaultCache       string
	DefaultQueue       string
	DefaultComposerVer string   // Default Composer version for this framework ("" = not applicable)
	Includes           []string // List of include files to load
}

// FrameworkConfigs is populated by framework registration during startup.
var FrameworkConfigs = map[string]FrameworkConfig{}

func GetFrameworkConfig(name string) (FrameworkConfig, bool) {
	config, ok := FrameworkConfigs[NormalizeFrameworkAlias(name)]
	return config, ok
}

// RegisterFrameworkConfig registers cfg as the FrameworkConfig for name,
// keyed the same way GetFrameworkConfig looks it up. Called from
// frameworks.Register (alongside RegisterDetection) so a framework
// package can own its own FrameworkConfig literal instead of an entry in
// this file's static FrameworkConfigs map. Not safe for concurrent calls;
// intended usage is registration during package init(), before
// GetFrameworkConfig is ever called.
func RegisterFrameworkConfig(name string, cfg FrameworkConfig) {
	FrameworkConfigs[strings.ToLower(strings.TrimSpace(name))] = cfg
}

func FrameworkUsesNodeRuntime(name string) bool {
	config, ok := GetFrameworkConfig(name)
	return ok && strings.EqualFold(config.Runtime, "node")
}

func FrameworkUsesPythonRuntime(name string) bool {
	config, ok := GetFrameworkConfig(name)
	return ok && strings.EqualFold(config.Runtime, "python")
}

func ResolveFrameworkAppService(name string) string {
	config, ok := GetFrameworkConfig(name)
	if ok && strings.TrimSpace(config.AppService) != "" {
		return config.AppService
	}
	return "php"
}

func ResolveFrameworkAppWorkdir(name string) string {
	config, ok := GetFrameworkConfig(name)
	if ok && strings.TrimSpace(config.AppWorkdir) != "" {
		return config.AppWorkdir
	}
	return conventions.DefaultWorkDir
}

// RequiresPHP returns true if the project requires a PHP container.
// It checks user config first (php_version), then falls back to framework defaults.
func RequiresPHP(config Config) bool {
	phpVersion := strings.TrimSpace(config.Stack.PHPVersion)
	// User explicitly set php_version to "none" → no PHP needed
	if phpVersion == "none" {
		return false
	}
	// User explicitly set php_version → requires PHP
	if phpVersion != "" {
		return true
	}
	// php_version not set → check framework's DefaultPHP
	fwConfig, ok := GetFrameworkConfig(config.Framework)
	if ok && fwConfig.DefaultPHP != "" {
		return true
	}
	return false
}
