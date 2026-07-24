package drupal

import "govard/internal/engine"

// manifest is Drupal's FrameworkManifestConfig literal - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "drupal" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored: []string{
		"cache_*",
		"watchdog",
		"sessions",
	},
	Sensitive: []string{
		"users_field_data",
		"users",
	},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "sites/default/files",
		RemoteMedia: "sites/default/files",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "web", Value: "/web"},
		},
	},
	Sync: engine.FrameworkSyncConfig{
		NoiseExcludes: []string{
			"cache_*",
			"watchdog",
			"sessions",
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 false,
	},
}
