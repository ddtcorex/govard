package openmage

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
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
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewOpenMageBootstrap(opts)
		},
	}
}
