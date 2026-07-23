package django

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("django")
	manifest, _ := engine.GetFrameworkManifestConfig("django")
	return types.FrameworkDefinition{
		Name:        "django",
		DisplayName: "Django",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			FilePaths: []string{"manage.py"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewDjangoBootstrap(opts)
		},
		SupportsFreshInstall: true,
		SupportsBootstrap:    true,
	}
}
