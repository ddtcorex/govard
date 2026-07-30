package magento2

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "magento2",
		Aliases:     []string{"magento"},
		DisplayName: "Magento 2",
		Config:      config,
		Manifest:    Manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"magento/product-community-edition", "magento/product-enterprise-edition", "magento/framework"},
			AuthJSONHosts:    []string{"repo.magento.com"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.Magento2Manager{}
		},
		FreshInstall:            freshInstall,
		PreConfigureHook:        PreConfigure,
		PostCloneHook:           PostClone,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
	}
}
