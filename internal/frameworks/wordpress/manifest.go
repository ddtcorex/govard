package wordpress

import "govard/internal/engine"

// manifest is WordPress's FrameworkManifestConfig literal - registered
// into engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "wordpress" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored: []string{
		"options_bak",
		"options_replica",
		"options_tmp",
		"redirection_404",
		"wflogs",
	},
	Sensitive: []string{
		"commentmeta",
		"comments",
		"usermeta",
		"users",
	},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "wp-content/uploads",
		RemoteMedia: "wp-content/uploads",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "wordpress", Value: "/wordpress"},
		},
	},
	Sync: engine.FrameworkSyncConfig{
		NoiseExcludes: []string{
			"wp-content/cache/",
			"wp-content/logs/",
		},
		MediaExcludes: engine.FrameworkMediaExcludeSet{
			NonAll:    []string{},
			Optimized: []string{},
			Minimal:   []string{},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 true,
	},
}
