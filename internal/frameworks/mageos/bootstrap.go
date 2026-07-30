package mageos

import (
	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento2"
)

// Variant is Mage-OS's magento2.FamilyVariant value - Mage-OS shares
// Magento 2's fresh-install/clone-workflow pipeline
// (internal/frameworks/magento2/bootstrap.go), parameterized by this
// value instead of duplicating the logic.
var Variant = magento2.FamilyVariant{
	Name:          "mageos",
	DisplayName:   "Mage-OS",
	RepositoryURL: "https://repo.mage-os.org",
	DBName:        conventions.DefaultMageOSDBName,
	DBUser:        conventions.DefaultMageOSDBUser,
	DBPass:        conventions.DefaultMageOSDBPass,
}

// NewBootstrap builds Mage-OS's bootstrapper.
func NewBootstrap(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
	return magento2.NewFamilyBootstrap(opts, Variant.Name, FreshCommands)
}

// FreshCommands is Mage-OS's own FreshCommands summary.
func FreshCommands(opts bootstrap.Options) []string {
	version := opts.Version
	if version == "" {
		version = "1.3.1"
	}
	return []string{
		"composer create-project mage-os/project-community-edition:" + version + " --repository-url=https://repo.mage-os.org .",
	}
}
