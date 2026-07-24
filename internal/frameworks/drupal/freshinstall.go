package drupal

import "govard/internal/engine/bootstrap"

// freshInstall runs Drupal's fresh-install sequence. Drupal has no
// bespoke steps beyond the generic CreateProject -> Install ->
// `govard config auto` sequence (its old entry in
// internal/cmd/bootstrap_fresh_install.go's genericFreshInstallFrameworks
// map was `"drupal": {}` - no DB, no domain), so it just delegates to
// the shared helper.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return bootstrap.GenericFreshInstall(NewDrupalBootstrap(opts), projectDir, helpers)
}
