package mageos

import "govard/internal/engine/bootstrap"

// freshInstall runs Mage-OS's fresh-install sequence by delegating to the
// shared Magento-family orchestrator (internal/engine/bootstrap/
// magento_family.go), parameterized by bootstrap.MageOSVariant.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.MagentoFamilyFreshInstall(bootstrap.MageOSVariant, opts, projectDir, helpers)
}
