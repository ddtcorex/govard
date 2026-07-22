package wordpress

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("wordpress")
	manifest, _ := engine.GetFrameworkManifestConfig("wordpress")
	return types.FrameworkDefinition{
		Name:        "wordpress",
		Aliases:     []string{"wp"},
		DisplayName: "WordPress",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"johnpbloch/wordpress", "roots/wordpress", "wordpress/wordpress"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewWordPressBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &tunnel.WordPressManager{}
		},
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
	}
}
