package laravel

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "laravel",
		DisplayName: "Laravel",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"laravel/framework"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewLaravelBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.LaravelManager{}
		},
		FreshInstall:         freshInstall,
		FreshInstallNeedsDB:  true,
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
