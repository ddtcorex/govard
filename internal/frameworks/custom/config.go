package custom

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

var config = engine.FrameworkConfig{
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
	},
}
