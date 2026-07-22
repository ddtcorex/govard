package emdash

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("emdash")
	manifest, _ := engine.GetFrameworkManifestConfig("emdash")
	return types.FrameworkDefinition{
		Name:        "emdash",
		DisplayName: "Emdash",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			PackageJSONDeps: []string{"emdash"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewEmdashBootstrap(opts)
		},
		SupportsFreshInstall: true,
	}
}
