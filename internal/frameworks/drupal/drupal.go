package drupal

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("drupal")
	manifest, _ := engine.GetFrameworkManifestConfig("drupal")
	return types.FrameworkDefinition{
		Name:        "drupal",
		DisplayName: "Drupal",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"drupal/core"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewDrupalBootstrap(opts)
		},
		SupportsFreshInstall: true,
	}
}
