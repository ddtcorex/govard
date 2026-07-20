package symfony

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("symfony")
	manifest, _ := engine.GetFrameworkManifestConfig("symfony")
	return types.FrameworkDefinition{
		Name:        "symfony",
		DisplayName: "Symfony",
		Config:      config,
		Manifest:    manifest,
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewSymfonyBootstrap(opts)
		},
	}
}
