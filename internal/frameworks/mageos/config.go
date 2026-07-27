package mageos

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// config is Mage-OS's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go. Byte-identical to Magento 2's
// config (internal/frameworks/magento2/config.go) except Name,
// DatabaseName, and DefaultPHP (one version behind Magento 2's) - not a
// simplification, the source map entries were themselves that close.
var config = engine.FrameworkConfig{
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
}
