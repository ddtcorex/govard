package openmage

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("openmage")
	manifest, _ := engine.GetFrameworkManifestConfig("openmage")
	return types.FrameworkDefinition{
		Name:        "openmage",
		DisplayName: "OpenMage",
		Config:      config,
		Manifest:    manifest,
		// Detect is intentionally the zero value: openmage has no
		// composer-package or file-path heuristic of its own today (see
		// magento1's Detect comment) - preserving current behavior exactly.
		Detect: engine.DetectionSpec{},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewOpenMageBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.Magento1Manager{}
		},
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
