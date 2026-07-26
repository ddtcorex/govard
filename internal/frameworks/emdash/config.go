package emdash

import "govard/internal/engine"

// config is Emdash's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go.
var config = engine.FrameworkConfig{
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
}
