package emdash

import "govard/internal/engine/bootstrap"

// freshInstall runs Emdash's fresh-install sequence: CreateProject ->
// Install -> Configure. No ConfigureAuto call - the old
// runBootstrapEmdashFreshInstall in internal/cmd/bootstrap_fresh_install.go
// never called it. Overrides opts.Runner to nil - the registry dispatcher
// wires opts.Runner to the PHP container runner by default, but Emdash's
// CreateProject never execs into any container (plain HTTP tarball
// download), matching the old switch-case function's emdashOpts, which
// never set Runner at all.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	opts.Runner = nil
	e := NewEmdashBootstrap(opts)

	if err := e.CreateProject(projectDir); err != nil {
		return err
	}
	if err := e.Install(projectDir); err != nil {
		return err
	}

	return e.Configure(projectDir)
}
