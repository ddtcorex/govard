package magento1

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// BuildConfig returns the shared Magento-1-family FrameworkConfig
// (Magento 1 and OpenMage), which differ only in name, database name,
// default PHP version, and default composer version. Shared with
// OpenMage (internal/frameworks/openmage/config.go calls this directly).
func BuildConfig(name, databaseName, defaultPHP, defaultComposerVer string) engine.FrameworkConfig {
	return engine.FrameworkConfig{
		Name:               name,
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "",
		NGINXTemplate:      "magento1.conf",
		DatabaseName:       databaseName,
		DefaultPHP:         defaultPHP,
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
		DefaultComposerVer: defaultComposerVer,
		Includes: []string{
			"includes/base.yml",
			"includes/redis.yml",
			"includes/elasticsearch.yml",
			"magento1/services.yml",
			"includes/rabbitmq.yml",
		},
	}
}

var config = BuildConfig("magento1", "magento", "8.1", "2.2")
