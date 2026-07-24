package cakephp

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "cakephp",
		DisplayName: "CakePHP",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"cakephp/cakephp"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewCakePHPBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
	}
}
