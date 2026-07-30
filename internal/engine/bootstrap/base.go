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

	// AdminCreate mirrors the --no-admin CLI flag's inverse
	// (BootstrapRuntimeOptions.AdminCreate in internal/cmd) - whether the
	// clone-workflow's PostCloneHook should create an admin user. Only
	// Magento 2/Mage-OS's PostCloneHook reads this today.
	AdminCreate bool

	// MetaPackage is the composer meta-package to create-project from
	// (e.g. "magento/project-community-edition" or, after the mageos
	// swap in runBootstrapFrameworkFreshInstall,
	// "mage-os/project-community-edition"). Only the Magento family's
	// FreshInstall reads this - every other framework's create-project
	// package name is a compile-time constant inside its own bootstrap.go.
	MetaPackage string
	// HyvaInstall/IncludeSample mirror the --hyva-install/--include-sample
	// CLI flags. Only the Magento family's FreshInstall reads these.
	HyvaInstall   bool
	IncludeSample bool

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

	// EnsureAuthJSON prompts for/writes repo.magento.com credentials if
	// needed (internal/cmd's ensureBootstrapAuthJSON) - interactive and
	// filesystem-touching, so it can't live in the framework package
	// without importing internal/cmd. Only the Magento family's
	// FreshInstall calls this.
	EnsureAuthJSON func() error
	// FixComposerCompatibility runs engine.FixComposerCompatibility for
	// the current project. Only the Magento family's FreshInstall calls
	// this today, but it's framework-agnostic in principle.
	FixComposerCompatibility func() error
	// RunHyvaInstall sets the Hyva composer token/repo and requires the
	// theme package (internal/cmd's runBootstrapHyvaInstall). Only called
	// when Options.HyvaInstall is true.
	RunHyvaInstall func() error
	// ResolveMagentoTablePrefix resolves/validates the table prefix from
	// config or the TABLE_PREFIX env var (internal/cmd's
	// resolveBootstrapMagentoTablePrefix).
	ResolveMagentoTablePrefix func() (string, error)
	// RunMagentoSetupInstall applies a best-effort Elasticsearch/OpenSearch
	// read-only-allow-delete fix (if the PHP container is already running)
	// and then runs `govard tool magento` with the given setup:install
	// args (internal/cmd's tail of runBootstrapPostInstall). The args
	// themselves are built by the caller via
	// bootstrap.BuildMagentoSetupInstallArgs (Task 2) - this closure only
	// executes them, since running a govard subcommand needs the
	// cmd-package-only *cobra.Command.
	RunMagentoSetupInstall func(args []string) error
	// RunMagentoSampleData runs sample:deploy/setup:upgrade/indexer:reindex/
	// cache:flush (internal/cmd's runBootstrapSampleData). Only called
	// when Options.IncludeSample is true.
	RunMagentoSampleData func() error

	// EnsureMagentoEnvPHP generates app/etc/env.php if missing, probing
	// the remote for a crypt key/table prefix to reuse where possible
	// (internal/cmd's ensureBootstrapMagentoEnvPHP). Only the Magento
	// family's PreConfigureHook calls this.
	EnsureMagentoEnvPHP func() error
	// RunMagentoAdminCreate creates the default Magento admin user via
	// `govard tool magento admin:user:create`, best-effort - it never
	// returns an error itself (internal/cmd's runBootstrapAdminCreate
	// only warns on failure), so this closure always returns nil. Only
	// the Magento family's PostCloneHook calls this, and only when
	// Options.AdminCreate is true.
	RunMagentoAdminCreate func() error
	// RunMagentoReindex runs `govard tool magento indexer:reindex`
	// (internal/cmd's runBootstrapMagentoReindex) - unlike
	// RunMagentoAdminCreate, a failure here does propagate as a real
	// error. Only the Magento family's PostCloneHook calls this.
	RunMagentoReindex func() error
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
