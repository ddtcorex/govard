package nextjs

import "govard/internal/engine/bootstrap"

// freshInstall runs Next.js's fresh-install sequence: CreateProject ->
// Configure. No Install() call - `create-next-app --use-npm` already
// installs dependencies while scaffolding, so a separate Install() step
// (needed by PostClone, whose cloned project didn't ship node_modules)
// would be redundant here. No ConfigureAuto call either - the old
// runBootstrapNextJSFreshInstall in
// internal/cmd/bootstrap_fresh_install.go never called it. Overrides
// opts.Runner to the Node container runner - the registry dispatcher
// wires opts.Runner to the PHP container runner by default, which is
// wrong for a Node framework.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	opts.Runner = helpers.NodeRunner
	n := NewNextJSBootstrap(opts)

	if err := n.CreateProject(projectDir); err != nil {
		return err
	}

	return n.Configure(projectDir)
}
