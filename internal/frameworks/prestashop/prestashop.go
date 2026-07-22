package prestashop

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("prestashop")
	manifest, _ := engine.GetFrameworkManifestConfig("prestashop")
	return types.FrameworkDefinition{
		Name:        "prestashop",
		DisplayName: "PrestaShop",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			FilePaths: []string{"config/defines.inc.php"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return bootstrap.NewPrestaShopBootstrap(opts)
		},
		SupportsBootstrap: true,
	}
}
