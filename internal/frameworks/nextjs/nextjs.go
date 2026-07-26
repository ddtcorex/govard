package nextjs

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "nextjs",
		DisplayName: "Next.js",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			PackageJSONDeps: []string{"next"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewNextJSBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
	}
}
