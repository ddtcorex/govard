package nextjs

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("nextjs")
	manifest, _ := engine.GetFrameworkManifestConfig("nextjs")
	return types.FrameworkDefinition{
		Name:        "nextjs",
		DisplayName: "Next.js",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			PackageJSONDeps: []string{"next"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewNextJSBootstrap(opts)
		},
		SupportsFreshInstall: true,
	}
}
