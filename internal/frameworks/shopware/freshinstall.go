package shopware

import "govard/internal/engine/bootstrap"

// freshInstall runs Shopware's fresh-install sequence. Shopware has no
// bespoke steps beyond the generic CreateProject -> Install ->
// `govard config auto` sequence (its old entry in
// internal/cmd/bootstrap_fresh_install.go's genericFreshInstallFrameworks
// map was `"shopware": {needsDomain: true}`), so it just delegates to the
// shared helper - the Domain field is already populated in opts by the
// caller, based on FreshInstallNeedsDomain in Definition().
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.GenericFreshInstall(NewShopwareBootstrap(opts), projectDir, helpers)
}
