package mageos

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
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
			return NewBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.Magento2Manager{}
		},
		FreshInstall:            freshInstall,
		PreConfigureHook:        magento2.PreConfigure,
		PostCloneHook:           magento2.PostClone,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
	}
}
