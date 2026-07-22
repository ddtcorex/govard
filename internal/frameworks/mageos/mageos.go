package mageos

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("mageos")
	manifest, _ := engine.GetFrameworkManifestConfig("mageos")
	return types.FrameworkDefinition{
		Name:        "mageos",
		DisplayName: "Mage-OS",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{
				"mage-os/product-community-edition",
				"mage-os/project-community-edition",
			},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewMageOSBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.Magento2Manager{}
		},
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
