package laravel

import "govard/internal/engine/bootstrap"

// freshInstall runs Laravel's fresh-install sequence. Laravel has no
// bespoke steps beyond the generic CreateProject -> Install ->
// `govard config auto` sequence (its old entry in
// internal/cmd/bootstrap_fresh_install.go's genericFreshInstallFrameworks
// map was `"laravel": {needsDB: true}`), so it just delegates to the
// shared helper - the DB fields are already populated in opts by the
// caller, based on FreshInstallNeedsDB in Definition().
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.GenericFreshInstall(NewLaravelBootstrap(opts), projectDir, helpers)
}
