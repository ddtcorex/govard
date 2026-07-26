package openmage

import (
	"fmt"
	"govard/internal/engine/bootstrap"
)

// freshInstall runs OpenMage's fresh-install sequence: CreateProject ->
// Install -> Configure. Deliberately does NOT call helpers.ConfigureAuto
// (`govard config auto`) - the old runBootstrapOpenMageFreshInstall in
// internal/cmd/bootstrap_fresh_install.go never called it either, so
// adding it here would be a behavior change, not a refactor.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	o := NewOpenMageBootstrap(opts)

	if err := o.CreateProject(projectDir); err != nil {
		return err
	}
	if err := o.Install(projectDir); err != nil {
		return err
	}
	if err := o.Configure(projectDir); err != nil {
		return fmt.Errorf("configure OpenMage: %w", err)
	}

	return nil
}
