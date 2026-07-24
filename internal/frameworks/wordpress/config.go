package wordpress

import (
	"govard/internal/conventions"
	"govard/internal/engine"
)

// config is WordPress's FrameworkConfig literal - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old static entry that used to live in
// internal/engine/framework_config.go.
var config = engine.FrameworkConfig{
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
}
