package magento1

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
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
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewMagento1Bootstrap(opts)
		},
	}
}
