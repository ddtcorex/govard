package cakephp

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("cakephp")
	manifest, _ := engine.GetFrameworkManifestConfig("cakephp")
	return types.FrameworkDefinition{
		Name:        "cakephp",
		DisplayName: "CakePHP",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"cakephp/cakephp"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewCakePHPBootstrap(opts)
		},
		SupportsFreshInstall: true,
	}
}
