package magento2

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// BuildConfig returns the shared Magento-2-family FrameworkConfig
// (Magento 2 and Mage-OS), which differ only in name, database name, and
// default PHP version - every other field was duplicated verbatim across
// both frameworks' config.go before this. Shared with Mage-OS
// (internal/frameworks/mageos/config.go calls this directly).
func BuildConfig(name, databaseName, defaultPHP string) engine.FrameworkConfig {
	return engine.FrameworkConfig{
		Name:               name,
		Runtime:            "php",
		AppService:         "php",
		AppWorkdir:         conventions.DefaultWorkDir,
		NGINXPUBLIC:        "/pub",
		NGINXTemplate:      "magento2.conf",
		DatabaseName:       databaseName,
		DefaultPHP:         defaultPHP,
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
		},
	}
}

var config = BuildConfig(Variant.Name, Variant.DBName, "8.5")
