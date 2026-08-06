package dagster

import "govard/internal/engine"

// config is Dagster's FrameworkConfig literal - registered into engine's
// runtime store by Definition() (via frameworks.Register). Mirrors
// Django's entry (internal/frameworks/django/config.go): Python runtime,
// no PHP-FPM/nginx block, Postgres by default.
var config = engine.FrameworkConfig{
	Name:               "dagster",
	Runtime:            "python",
	AppService:         "web",
	AppWorkdir:         "/app",
	NGINXPUBLIC:        "",
	NGINXTemplate:      "",
	DatabaseName:       "dagster",
	DefaultPHP:         "",
	DefaultPythonVer:   "3.12",
	DefaultDB:          "postgres",
	DefaultDBVer:       "16",
	DefaultWebServer:   "none",
	DefaultSearch:      "none",
	DefaultCache:       "none",
	DefaultQueue:       "none",
	DefaultComposerVer: "",
	Includes: []string{
		"dagster/services.yml",
	},
}
