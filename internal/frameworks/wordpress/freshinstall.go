package wordpress

import "govard/internal/engine/bootstrap"

// freshInstall runs WordPress's fresh-install sequence. WordPress has no
// bespoke steps beyond the generic CreateProject -> Install ->
// `govard config auto` sequence (its old entry in
// internal/cmd/bootstrap_fresh_install.go's genericFreshInstallFrameworks
// map was `"wordpress": {needsDB: true, needsDomain: true}`), so it just
// delegates to the shared helper - the DB and Domain fields are already
// populated in opts by the caller, based on FreshInstallNeedsDB/
// FreshInstallNeedsDomain in Definition().
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.GenericFreshInstall(NewWordPressBootstrap(opts), projectDir, helpers)
}
