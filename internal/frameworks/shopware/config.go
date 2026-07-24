package shopware

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// config is Shopware's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go.
var config = engine.FrameworkConfig{
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
}
