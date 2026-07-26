package django

import "govard/internal/engine"

// manifest is Django's FrameworkManifestConfig literal - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "django" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. No
// "sync" key in the source JSON entry, so Sync is left at its zero value
// here too (matching what json.Unmarshal produces for an absent key).
var manifest = engine.FrameworkManifestConfig{
	Ignored:   []string{},
	Sensitive: []string{},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:        "media",
		RemoteMedia:       "media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: false,
		SupportsPostClone:                 true,
	},
}
