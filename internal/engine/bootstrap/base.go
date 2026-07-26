package bootstrap

import "errors"

type Options struct {
	Source   string
	Clone    bool
	CodeOnly bool
	Fresh    bool
	Version  string
	Env      string
	Runner   func(command string) error

	// SkipUp mirrors the --no-up CLI flag (BootstrapRuntimeOptions.SkipUp in
	// internal/cmd). Only Django's FreshInstall reads this today - it must
	// scaffold the project, bring the environment up itself, then run
	// Install()/migrate against the running container, so it needs to know
	// whether the caller asked to skip that "bring the environment up"
	// step entirely.
	SkipUp bool

	// Database credentials for local configuration
	DBHost      string
	DBUser      string
	DBPass      string
	DBName      string
	TablePrefix string

	// Environment configuration
	Domain      string
	ProjectName string

	// PrestaShop encryption secrets carried over from a remote's parameters.php, so a
	// fabricated local parameters.php can reuse them instead of generating fresh ones
	// (module data encrypted under the remote's keys would otherwise be undecryptable
	// after a DB clone). Left empty when no remote secrets were available/probed.
	PrestaShopSecret       string
	PrestaShopCookieKey    string
	PrestaShopCookieIV     string
	PrestaShopNewCookieKey string
}

// CmdHelpers bundles the cmd-package-level closures a framework's
// FreshInstall function needs, so internal/frameworks/<name> packages can
// orchestrate their own fresh-install sequence without importing
// internal/cmd (which would create an import cycle - internal/cmd already
// imports internal/frameworks, which imports this package).
type CmdHelpers struct {
	// ConfigureAuto runs `govard config auto` against the project - the
	// step every fresh-install sequence ends with before printing success.
	ConfigureAuto func() error

	// NodeRunner/PythonRunner exec a shell command inside a throwaway
	// Node/Python container (internal/cmd's nodeCreateProjectRunner/
	// pythonCreateProjectRunner), for frameworks whose FreshInstall needs a
	// different runtime than the PHP container the caller wires into
	// Options.Runner by default. A framework's freshInstall function
	// overrides opts.Runner with one of these before constructing its
	// bootstrapper - e.g. Next.js sets opts.Runner = helpers.NodeRunner.
	NodeRunner   func(command string) error
	PythonRunner func(command string) error

	// EnvUp runs `govard env up --remove-orphans` - the step Django's
	// FreshInstall must run itself, between CreateProject and Install,
	// since its compose "web" container runs `python manage.py runserver`
	// directly and can't come up against an empty/unmigrated project.
	EnvUp func() error
}

// ErrFreshInstallSkipUp is returned by a framework's FreshInstall function
// to signal "fresh install completed, but env-up/Install were
// intentionally skipped because Options.SkipUp was set" - the caller
// (internal/cmd's runBootstrapRegistryFreshInstall) treats it as success
// but skips the generic "Fresh <Framework> bootstrap completed." message,
// matching the pre-migration behavior where the equivalent early return
// happened before that message was ever printed.
var ErrFreshInstallSkipUp = errors.New("fresh install completed with env up skipped")

func DefaultOptions() Options {
	return Options{Source: "staging"}
}

type FrameworkBootstrap interface {
	Name() string
	SupportsFreshInstall() bool
	SupportsClone() bool
	FreshCommands() []string
	CreateProject(projectDir string) error
	Install(projectDir string) error
	Configure(projectDir string) error
	PostClone(projectDir string) error
}

func Magento2FreshCommands(opts Options) []string {
	version := opts.Version
	if version == "" {
		version = "2.4.8"
	}
	return []string{
		"composer create-project magento/project-community-edition:" + version + " .",
	}
}

func MageOSFreshCommands(opts Options) []string {
	version := opts.Version
	if version == "" {
		version = "1.3.1"
	}
	return []string{
		"composer create-project mage-os/project-community-edition:" + version + " --repository-url=https://repo.mage-os.org .",
	}
}
