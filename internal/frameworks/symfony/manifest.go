package symfony

import "govard/internal/engine"

// manifest is Symfony's FrameworkManifestConfig literal - registered
// into engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "symfony" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored: []string{
		"messenger_messages",
	},
	Sensitive: []string{
		"user",
	},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "public/media",
		RemoteMedia: "public/media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "public", Value: "/public"},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 true,
	},
}
