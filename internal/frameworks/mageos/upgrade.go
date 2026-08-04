package mageos

import (
	"context"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

// UpgradeVariant is Mage-OS's magento2.UpgradeVariant value, moved verbatim
// from engine.mageOSUpgradeVariant.
var UpgradeVariant = magento2.UpgradeVariant{
	DisplayName:   "Mage-OS",
	Metapackage:   "mage-os/project-community-edition",
	RepositoryURL: "https://repo.mage-os.org/",
	PackagePrefix: "mage-os/",
}

// Upgrade is Mage-OS's engine.UpgradeFunc - delegates to magento2's shared
// family pipeline, parameterized by Mage-OS's own variant, the same way
// mageos.freshInstall delegates to magento2.FreshInstall(Variant, ...).
func Upgrade(ctx context.Context, config engine.Config, opts engine.UpgradeOptions) error {
	return magento2.RunUpgrade(ctx, config, opts, UpgradeVariant)
}
