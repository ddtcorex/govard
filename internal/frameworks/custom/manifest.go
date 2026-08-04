package custom

import "govard/internal/engine"

var manifest = engine.FrameworkManifestConfig{
	Ignored:   []string{},
	Sensitive: []string{},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:        "public/media",
		RemoteMedia:       "public/media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 false,
	},
}
