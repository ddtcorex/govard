package django

import "govard/internal/engine"

// config is Django's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go.
var config = engine.FrameworkConfig{
	Name:               "django",
	Runtime:            "python",
	AppService:         "web",
	AppWorkdir:         "/app",
	NGINXPUBLIC:        "",
	NGINXTemplate:      "",
	DatabaseName:       "django",
	DefaultPHP:         "",
	DefaultPythonVer:   "3.12",
	DefaultDB:          "postgres",
	DefaultDBVer:       "16",
	DefaultNginxVer:    "1.28",
	DefaultApacheVer:   "2.4",
	DefaultQueueVer:    "4.2",
	DefaultWebServer:   "none",
	DefaultSearch:      "none",
	DefaultCache:       "none",
	DefaultQueue:       "none",
	DefaultComposerVer: "",
	Includes: []string{
		"django/services.yml",
	},
}
