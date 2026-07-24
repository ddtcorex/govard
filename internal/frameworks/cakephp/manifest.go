package cakephp

import "govard/internal/engine"

// manifest is CakePHP's FrameworkManifestConfig literal - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "cakephp" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
//
// requires_running_env_for_fresh_install is true for CakePHP (unlike most
// generic-fresh-install frameworks): its fresh-install commands exec into
// the already-running PHP container, so `govard bootstrap --fresh` must
// bring the environment up before running CreateProject for this
// framework. That ordering decision is made elsewhere (based on this
// flag, via engine.FrameworkRequiresRunningEnvForFreshInstall) and is
// unchanged by this migration.
var manifest = engine.FrameworkManifestConfig{
	Ignored:   []string{},
	Sensitive: []string{},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:  "public/media",
		RemoteMedia: "public/media",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{
			{Path: "public", Value: "/public"},
			{Path: "webroot", Value: "/webroot"},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 false,
	},
}
