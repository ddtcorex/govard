package magento2

import "govard/internal/engine/bootstrap"

// freshInstall runs Magento 2's fresh-install sequence by delegating to
// the shared Magento-family orchestrator (internal/engine/bootstrap/
// magento_family.go), parameterized by bootstrap.Magento2Variant.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.MagentoFamilyFreshInstall(bootstrap.Magento2Variant, opts, projectDir, helpers)
}
