package laravel

import "govard/internal/engine"

// manifest is Laravel's FrameworkManifestConfig literal - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "laravel" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored: []string{
		"cache",
		"cache_locks",
		"failed_jobs",
		"job_batches",
		"jobs",
		"sessions",
		"telescope_entries",
		"telescope_entries_tags",
		"telescope_monitoring",
	},
	Sensitive: []string{
		"password_reset_tokens",
		"password_resets",
		"personal_access_tokens",
		"users",
	},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "public/media",
		RemoteMedia: "public/media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "public", Value: "/public"},
		},
	},
	Sync: engine.FrameworkSyncConfig{
		NoiseExcludes: []string{
			"storage/framework/cache/data/*",
			"storage/framework/sessions/*",
			"storage/framework/views/*",
			"storage/logs/*",
		},
		MediaExcludes: engine.FrameworkMediaExcludeSet{
			NonAll:    []string{"testing/"},
			Optimized: []string{},
			Minimal:   []string{},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 true,
	},
}
