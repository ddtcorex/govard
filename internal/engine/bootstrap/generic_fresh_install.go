package bootstrap

import "fmt"

// GenericFreshInstall runs the CreateProject -> Install -> ConfigureAuto
// sequence shared by frameworks with no bespoke fresh-install steps - the
// framework-owned equivalent of
// internal/cmd/bootstrap_fresh_install.go's
// runBootstrapGenericFreshInstall, called from a framework's own
// FreshInstall function instead of from a central dispatcher switch.
func GenericFreshInstall(fwBootstrap FrameworkBootstrap, projectDir string, helpers CmdHelpers) error {
	if err := fwBootstrap.CreateProject(projectDir); err != nil {
		return err
	}
	if err := fwBootstrap.Install(projectDir); err != nil {
		return err
	}
	if err := helpers.ConfigureAuto(); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}
	return nil
}
