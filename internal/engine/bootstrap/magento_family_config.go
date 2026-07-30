package bootstrap

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// BuildMagento2FamilyConfig returns the shared Magento-2-family
// FrameworkConfig (Magento 2 and Mage-OS), which differ only in name,
// database name, and default PHP version - every other field was
// duplicated verbatim across both frameworks' config.go before this.
func BuildMagento2FamilyConfig(name, databaseName, defaultPHP string) engine.FrameworkConfig {
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
			"includes/livereload.yml",
		},
	}
}

// BuildMagento1FamilyConfig returns the shared Magento-1-family
// FrameworkConfig (Magento 1 and OpenMage), which differ only in name,
// database name, default PHP version, and default composer version.
func BuildMagento1FamilyConfig(name, databaseName, defaultPHP, defaultComposerVer string) engine.FrameworkConfig {
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
