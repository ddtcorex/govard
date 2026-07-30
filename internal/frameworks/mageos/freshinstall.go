package mageos

import (
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento2"
)

// freshInstall runs Mage-OS's fresh-install sequence by delegating to the
// shared Magento-family orchestrator (internal/frameworks/magento2/
// bootstrap.go), parameterized by Variant.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return magento2.FreshInstall(Variant, opts, projectDir, helpers)
}
