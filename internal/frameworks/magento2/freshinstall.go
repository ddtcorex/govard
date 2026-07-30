package magento2

import "govard/internal/engine/bootstrap"

// freshInstall runs Magento 2's fresh-install sequence by delegating to
// the shared Magento-family orchestrator (bootstrap.go in this package),
// parameterized by Variant.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return FreshInstall(Variant, opts, projectDir, helpers)
}
