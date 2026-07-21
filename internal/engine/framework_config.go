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

// FrameworkConfigs maps framework names to their configurations
var FrameworkConfigs = map[string]FrameworkConfig{
	"magento2": {
		Name:               "magento2",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/pub",
		NGINXTemplate:      "magento2.conf",
		DatabaseName:       "magento",
		DefaultPHP:         "8.5",
		DefaultNodeVer:     "24",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.8",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "opensearch",
		DefaultCache:       "redis",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"includes/varnish.yml",
			"includes/rabbitmq.yml",
			"includes/selenium.yml",
			"includes/livereload.yml",
		},
	},
	"mageos": {
		Name:               "mageos",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/pub",
		NGINXTemplate:      "magento2.conf",
		DatabaseName:       "mageos",
		DefaultPHP:         "8.4",
		DefaultNodeVer:     "24",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.8",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "opensearch",
		DefaultCache:       "redis",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"includes/varnish.yml",
			"includes/rabbitmq.yml",
			"includes/selenium.yml",
			"includes/livereload.yml",
		},
	},
	"laravel": {
		Name:               "laravel",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/public",
		NGINXTemplate:      "laravel.conf",
		DatabaseName:       "laravel",
		DefaultPHP:         "8.4",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"laravel/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"nextjs": {
		Name:               "nextjs",
		Runtime:            "node",
		AppService:         "web",
		AppWorkdir:         "/app",
		NGINXPUBLIC:        "",
		NGINXTemplate:      "nodejs.conf",
		DatabaseName:       "",
		DefaultPHP:         "",
		DefaultNodeVer:     "24",
		DefaultDB:          "none",
		DefaultDBVer:       "",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "none",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "",
		Includes: []string{
			"nextjs/services.yml",
			"includes/redis.yml",
			"includes/rabbitmq.yml",
		},
	},
	"emdash": {
		Name:               "emdash",
		Runtime:            "node",
		AppService:         "web",
		AppWorkdir:         "/app",
		NGINXPUBLIC:        "",
		NGINXTemplate:      "",
		DatabaseName:       "",
		DefaultPHP:         "",
		DefaultNodeVer:     "22",
		DefaultDB:          "none",
		DefaultDBVer:       "",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "none",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "",
		Includes: []string{
			"emdash/services.yml",
		},
	},
	"drupal": {
		Name:               "drupal",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/web",
		NGINXTemplate:      "drupal.conf",
		DatabaseName:       "drupal",
		DefaultPHP:         "8.4",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"includes/rabbitmq.yml",
		},
	},
	"symfony": {
		Name:               "symfony",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/public",
		NGINXTemplate:      "symfony.conf",
		DatabaseName:       "symfony",
		DefaultPHP:         "8.4",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"symfony/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"magento1": {
		Name:               "magento1",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "magento1.conf",
		DatabaseName:       "magento",
		DefaultPHP:         "8.1",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "10.11",
		DefaultMySQLVer:    "8.0",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.0",
		DefaultSearchVer:   "1.3",
		DefaultVarnishVer:  "6.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "2.2",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"magento1/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"openmage": {
		Name:               "openmage",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "magento1.conf",
		DatabaseName:       "openmage",
		DefaultPHP:         "8.2",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "10.11",
		DefaultMySQLVer:    "8.0",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.0",
		DefaultSearchVer:   "1.3",
		DefaultVarnishVer:  "6.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"magento1/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"shopware": {
		Name:               "shopware",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/public",
		NGINXTemplate:      "shopware.conf",
		DatabaseName:       "shopware",
		DefaultPHP:         "8.4",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"shopware/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"cakephp": {
		Name:               "cakephp",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/webroot",
		NGINXTemplate:      "cakephp.conf",
		DatabaseName:       "cakephp",
		DefaultPHP:         "8.4",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/rabbitmq.yml",
		},
	},
	"wordpress": {
		Name:               "wordpress",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "wordpress.conf",
		DatabaseName:       "wordpress",
		DefaultPHP:         "8.3",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "11.4",
		DefaultMySQLVer:    "8.4",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/rabbitmq.yml",
		},
	},
	"prestashop": {
		Name:               "prestashop",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "prestashop.conf",
		DatabaseName:       "prestashop",
		DefaultPHP:         "8.1",
		DefaultDB:          "mariadb",
		DefaultDBVer:       "10.11",
		DefaultMySQLVer:    "8.0",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "latest",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"prestashop/services.yml",
			"includes/rabbitmq.yml",
		},
	},
	"custom": {
		Name:               "custom",
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "default.conf",
		DatabaseName:       "app",
		DefaultPHP:         "",
		DefaultNodeVer:     "",
		DefaultDB:          "none",
		DefaultDBVer:       "",
		DefaultMySQLVer:    "",
		DefaultNginxVer:    "1.28",
		DefaultApacheVer:   "2.4",
		DefaultCacheVer:    "7.4",
		DefaultSearchVer:   "3.0",
		DefaultVarnishVer:  "8.0",
		DefaultQueueVer:    "4.2",
		DefaultWebServer:   "nginx",
		DefaultSearch:      "none",
		DefaultCache:       "none",
		DefaultQueue:       "none",
		DefaultComposerVer: "",
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"includes/varnish.yml",
			"includes/rabbitmq.yml",
			"includes/livereload.yml",
		},
	},
}

func GetFrameworkConfig(name string) (FrameworkConfig, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "magento" {
		name = "magento2"
	}
	config, ok := FrameworkConfigs[name]
	return config, ok
}

func FrameworkUsesNodeRuntime(name string) bool {
	config, ok := GetFrameworkConfig(name)
	return ok && strings.EqualFold(config.Runtime, "node")
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
