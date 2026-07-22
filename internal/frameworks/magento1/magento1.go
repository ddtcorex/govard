package magento1

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("magento1")
	manifest, _ := engine.GetFrameworkManifestConfig("magento1")
	return types.FrameworkDefinition{
		Name:        "magento1",
		DisplayName: "Magento 1",
		Config:      config,
		Manifest:    manifest,
		// ComposerPackages intentionally includes openmage/magento-lts and
		// magento-hackathon/magento-composer-installer - this is the exact,
		// pre-existing behavior of internal/engine/discovery.go (a project
		// using openmage/magento-lts is auto-detected as "magento1", not
		// "openmage"; openmage has no detection heuristic of its own).
		// This looks like it could be a bug, but changing it is out of
		// scope - Global Constraints require zero detection behavior change.
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"openmage/magento-lts", "magento-hackathon/magento-composer-installer"},
			FilePaths:        []string{"app/Mage.php", "app/etc/local.xml"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewMagento1Bootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.Magento1Manager{}
		},
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
