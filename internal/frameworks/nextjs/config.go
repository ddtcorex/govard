package nextjs

import "govard/internal/engine"

// config is Next.js's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go.
var config = engine.FrameworkConfig{
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
}
