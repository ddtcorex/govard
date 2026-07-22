package shopware

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("shopware")
	manifest, _ := engine.GetFrameworkManifestConfig("shopware")
	return types.FrameworkDefinition{
		Name:        "shopware",
		DisplayName: "Shopware",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"shopware/core", "shopware/platform"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewShopwareBootstrap(opts)
		},
		SupportsFreshInstall: true,
	}
}
