package openmage

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "openmage",
		DisplayName: "OpenMage",
		Config:      config,
		Manifest:    manifest,
		// Detect is intentionally the zero value - OpenMage has no
		// detection heuristic of its own. A project using
		// openmage/magento-lts is auto-detected as "magento1", not
		// "openmage" (see internal/frameworks/magento1/magento1.go's
		// Detect.ComposerPackages comment). Pre-existing behavior,
		// unchanged by this migration.
		Detect: engine.DetectionSpec{},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewOpenMageBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &magento1.Magento1Manager{}
		},
		FreshInstall:            freshInstall,
		FreshInstallNeedsDB:     true,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
	}
}
