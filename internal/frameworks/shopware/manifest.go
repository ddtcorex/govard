package shopware

import "govard/internal/engine"

// manifest is Shopware's FrameworkManifestConfig literal - registered
// into engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "shopware" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored:   []string{},
	Sensitive: []string{},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "public/media",
		RemoteMedia: "public/media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "public", Value: "/public"},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 false,
	},
}
