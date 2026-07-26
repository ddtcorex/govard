package django

import (
	"fmt"
	"govard/internal/engine/bootstrap"

	"github.com/pterm/pterm"
)

// freshInstall runs Django's fresh-install sequence: CreateProject -> (if
// not skipping up) EnvUp -> Install/migrate. Overrides opts.Runner to the
// Python container runner - the registry dispatcher wires opts.Runner to
// the PHP container runner by default, which is wrong for a Python
// framework. Django manages its own env-up timing (see
// frameworkFreshInstallManagesOwnEnvUp in internal/cmd/bootstrap_fresh_install.go,
// unchanged by this migration) because its compose "web" container runs
// `python manage.py runserver` directly and can't come up against an
// empty/unmigrated project - CreateProject must scaffold the project
// before the environment starts.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	opts.Runner = helpers.PythonRunner
	d := NewDjangoBootstrap(opts)

	if err := d.CreateProject(projectDir); err != nil {
		return err
	}

	if opts.SkipUp {
		pterm.Info.Println("Skipping env up and migrate (--no-up); run `govard env up` then `govard tool manage migrate` manually.")
		return bootstrap.ErrFreshInstallSkipUp
	}

	if err := helpers.EnvUp(); err != nil {
		return fmt.Errorf("failed to start local environment: %w", err)
	}

	return d.Install(projectDir)
}
