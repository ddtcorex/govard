package magento2

import (
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

// Definition returns magento2's FrameworkDefinition. Bootstrap is
// intentionally left nil: magento2 uses the free function
// bootstrap.Magento2FreshCommands instead of implementing
// bootstrap.FrameworkBootstrap, unlike every other framework - resolved
// when the bootstrap dispatchers are unified in a later migration step.
func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("magento2")
	manifest, _ := engine.GetFrameworkManifestConfig("magento2")
	return types.FrameworkDefinition{
		Name:        "magento2",
		Aliases:     []string{"magento"},
		DisplayName: "Magento 2",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"magento/product-community-edition", "magento/product-enterprise-edition", "magento/framework"},
			AuthJSONHosts:    []string{"repo.magento.com"},
		},
	}
}
