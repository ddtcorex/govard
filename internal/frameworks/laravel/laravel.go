package laravel

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("laravel")
	manifest, _ := engine.GetFrameworkManifestConfig("laravel")
	return types.FrameworkDefinition{
		Name:        "laravel",
		DisplayName: "Laravel",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"laravel/framework"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewLaravelBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.LaravelManager{}
		},
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
