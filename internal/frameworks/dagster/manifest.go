package dagster

import "govard/internal/engine"

// manifest is Dagster's FrameworkManifestConfig literal, registered into
// engine's runtime store by Definition() (via frameworks.Register).
// "tmp_*" excludes Dagster's per-run compute-log temp directories from
// govard sync; there is no media-asset concept to configure (no
// LocalMedia/RemoteMedia), matching Django's manifest shape.
var manifest = engine.FrameworkManifestConfig{
	Ignored:   []string{"__pycache__", ".venv", "tmp_*"},
	Sensitive: []string{},
	Paths: engine.FrameworkPathConfig{
		WebRootCandidates: []engine.FrameworkWebRootCandidate{},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: false,
		SupportsPostClone:                 true,
	},
}
