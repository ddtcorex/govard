package mageos

import (
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

// Definition returns mageos's FrameworkDefinition. Like magento2, Bootstrap
// is left nil: mageos reuses magento2's bespoke fresh-install orchestration
// (internal/cmd/bootstrap_fresh_install.go's runBootstrapFreshInstall,
// parameterized by repository URL/metapackage) rather than the
// bootstrap.FrameworkBootstrap interface, so there is no
// bootstrap.NewMageOSBootstrap to wrap here.
func Definition() types.FrameworkDefinition {
	config, _ := engine.GetFrameworkConfig("mageos")
	manifest, _ := engine.GetFrameworkManifestConfig("mageos")
	return types.FrameworkDefinition{
		Name:        "mageos",
		DisplayName: "Mage-OS",
		Config:      config,
		Manifest:    manifest,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{
				"mage-os/product-community-edition",
				"mage-os/project-community-edition",
			},
		},
	}
}
