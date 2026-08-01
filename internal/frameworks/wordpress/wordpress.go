package wordpress

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
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
			return NewWordPressBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &WordPressManager{}
		},
		FreshInstall:            freshInstall,
		FreshInstallNeedsDB:     true,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
	}
}
