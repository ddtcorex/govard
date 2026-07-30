package magento2

import (
	"fmt"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
)

// FamilyVariant parameterizes the shared Magento-2-family fresh-install/
// clone-workflow pipeline for a specific distribution - the same pattern
// internal/engine/upgrade_magento2.go's magentoUpgradeVariant already uses
// for `govard upgrade`. Magento 2 owns this type and the generic logic
// built on it below, since Magento 2 is the real, primary implementation;
// a sibling distribution (currently only Mage-OS, in
// internal/frameworks/mageos/bootstrap.go) imports this package and
// supplies its own FamilyVariant value instead of duplicating the logic.
type FamilyVariant struct {
	Name          string
	DisplayName   string
	RepositoryURL string
	DBName        string
	DBUser        string
	DBPass        string
}

// Variant is Magento 2's own FamilyVariant value.
var Variant = FamilyVariant{
	Name:          "magento2",
	DisplayName:   "Magento 2",
	RepositoryURL: "https://repo.magento.com",
	DBName:        conventions.DefaultMagentoDBName,
	DBUser:        conventions.DefaultMagentoDBUser,
	DBPass:        conventions.DefaultMagentoDBPass,
}

// familyBootstrap adapts a variant's FreshCommands function (a plain
// function, unlike every other framework's FrameworkBootstrap struct) to
// the bootstrap.FrameworkBootstrap interface, so the registry's Bootstrap
// factory field can be populated for Magento 2 and its siblings too. Only
// FreshCommands is real: the actual fresh-install orchestration is
// FreshInstall (below), reached via each distribution's own
// Definition().FreshInstall, not this generic interface, so the other
// lifecycle methods report that explicitly instead of silently no-op'ing.
type familyBootstrap struct {
	options       bootstrap.Options
	name          string
	freshCommands func(bootstrap.Options) []string
}

// NewBootstrap builds Magento 2's own bootstrapper.
func NewBootstrap(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
	return &familyBootstrap{options: opts, name: Variant.Name, freshCommands: FreshCommands}
}

// NewFamilyBootstrap builds a sibling distribution's bootstrapper (e.g.
// Mage-OS's), parameterized by its own name and FreshCommands function.
func NewFamilyBootstrap(opts bootstrap.Options, name string, freshCommands func(bootstrap.Options) []string) bootstrap.FrameworkBootstrap {
	return &familyBootstrap{options: opts, name: name, freshCommands: freshCommands}
}

func (m *familyBootstrap) Name() string {
	return m.name
}

func (m *familyBootstrap) SupportsFreshInstall() bool {
	return true
}

func (m *familyBootstrap) SupportsClone() bool {
	return true
}

func (m *familyBootstrap) FreshCommands() []string {
	return m.freshCommands(m.options)
}

func (m *familyBootstrap) unsupported(step string) error {
	return fmt.Errorf("%s %s is orchestrated by 'govard bootstrap --fresh' (internal/frameworks/%s/freshinstall.go), not bootstrap.FrameworkBootstrap", m.name, step, m.name)
}

func (m *familyBootstrap) CreateProject(projectDir string) error {
	return m.unsupported("project creation")
}

func (m *familyBootstrap) Install(projectDir string) error {
	return m.unsupported("installation")
}

func (m *familyBootstrap) Configure(projectDir string) error {
	return m.unsupported("configuration")
}

func (m *familyBootstrap) PostClone(projectDir string) error {
	return m.unsupported("post-clone setup")
}

// FreshCommands is Magento 2's own FreshCommands summary.
func FreshCommands(opts bootstrap.Options) []string {
	version := opts.Version
	if version == "" {
		version = "2.4.8"
	}
	return []string{
		"composer create-project magento/project-community-edition:" + version + " .",
	}
}

// BuildFreshCreateProjectCommand builds the shell command for a fresh
// composer create-project against variant's repository - moved verbatim
// from internal/cmd/bootstrap_fresh_install.go's
// bootstrapFreshCreateProjectCommandLine, parameterized by variant instead
// of a framework-name string check.
func BuildFreshCreateProjectCommand(variant FamilyVariant, opts bootstrap.Options) string {
	versionPart := ""
	if opts.Version != "" {
		versionPart = " " + engine.ShellQuote(opts.Version)
	}
	return strings.Join([]string{
		"set -e",
		"rm -rf /tmp/govard-create-project",
		"composer create-project -n --ignore-platform-reqs --repository-url=" + variant.RepositoryURL + " " +
			engine.ShellQuote(opts.MetaPackage) + " /tmp/govard-create-project" + versionPart,
		"if command -v rsync >/dev/null 2>&1; then rsync -a /tmp/govard-create-project/ " + conventions.DefaultWorkDir + "/; else cp -a /tmp/govard-create-project/. " + conventions.DefaultWorkDir + "/; fi",
		"rm -rf /tmp/govard-create-project",
	}, " && ")
}

// BuildSetupInstallArgs builds the `bin/magento setup:install` argument
// list for variant - moved verbatim from internal/cmd/
// bootstrap_post_install.go's runBootstrapPostInstall (the "build
// setupArgs" half; the "run it" half is now
// internal/cmd/bootstrap_fresh_install.go's runBootstrapMagentoSetupInstall,
// wired through CmdHelpers.RunMagentoSetupInstall).
//
// The legacy elasticsearch7-vs-opensearch version gate is intentionally
// magento2-only (variant.Name == "magento2"), matching the pre-existing
// behavior exactly - Mage-OS versions always get OpenSearch regardless of
// version, which is a known, pre-existing asymmetry this migration does
// not change.
func BuildSetupInstallArgs(variant FamilyVariant, version string, adminEmail string, tablePrefix string) []string {
	setupArgs := []string{
		"setup:install",
		"--backend-frontname=" + conventions.DefaultAdminPath,
		"--db-host=" + conventions.DefaultMagentoDBHost,
		"--db-name=" + variant.DBName,
		"--db-user=" + variant.DBUser,
		"--db-password=" + variant.DBPass,
		"--db-prefix=" + tablePrefix,
		"--search-engine=opensearch",
		"--opensearch-host=elasticsearch",
		"--opensearch-port=9200",
		"--opensearch-index-prefix=magento2",
		"--opensearch-enable-auth=0",
		"--opensearch-timeout=15",
		"--admin-user=" + conventions.DefaultAdminUser,
		"--admin-password=" + conventions.DefaultAdminPassword,
		"--admin-firstname=Admin",
		"--admin-lastname=User",
		"--admin-email=" + adminEmail,
	}

	if variant.Name == "magento2" && version != "" {
		if comparison, comparable := engine.CompareNumericDotVersions(version, "2.4.8"); comparable && comparison < 0 {
			setupArgs = []string{
				"setup:install",
				"--backend-frontname=" + conventions.DefaultAdminPath,
				"--db-host=" + conventions.DefaultMagentoDBHost,
				"--db-name=" + variant.DBName,
				"--db-user=" + variant.DBUser,
				"--db-password=" + variant.DBPass,
				"--db-prefix=" + tablePrefix,
				"--search-engine=elasticsearch7",
				"--elasticsearch-host=elasticsearch",
				"--elasticsearch-port=9200",
				"--elasticsearch-index-prefix=magento2",
				"--elasticsearch-enable-auth=0",
				"--elasticsearch-timeout=15",
				"--admin-user=" + conventions.DefaultAdminUser,
				"--admin-password=" + conventions.DefaultAdminPassword,
				"--admin-firstname=Admin",
				"--admin-lastname=User",
				"--admin-email=" + adminEmail,
			}
		}
	}

	return setupArgs
}

// FreshInstall runs the shared Magento 2/Mage-OS fresh-install sequence:
// EnsureAuthJSON -> FixComposerCompatibility -> create-project -> (if
// HyvaInstall) Hyva theme install -> setup:install -> `govard config auto`
// -> (if IncludeSample) sample data. Moved from internal/cmd/
// bootstrap_fresh_install.go's runBootstrapFreshInstall, parameterized by
// variant instead of an engine.Magento2FamilyDisplayName string check.
func FreshInstall(variant FamilyVariant, opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	if err := helpers.EnsureAuthJSON(); err != nil {
		return err
	}
	if err := helpers.FixComposerCompatibility(); err != nil {
		return err
	}

	commandLine := BuildFreshCreateProjectCommand(variant, opts)
	if err := opts.Runner(commandLine); err != nil {
		return fmt.Errorf("fresh create-project failed: %w", err)
	}

	if opts.HyvaInstall {
		if err := helpers.RunHyvaInstall(); err != nil {
			return err
		}
	}

	tablePrefix, err := helpers.ResolveMagentoTablePrefix()
	if err != nil {
		return err
	}
	adminEmail := conventions.AdminEmailForDomain(opts.Domain)
	setupArgs := BuildSetupInstallArgs(variant, opts.Version, adminEmail, tablePrefix)
	if err := helpers.RunMagentoSetupInstall(setupArgs); err != nil {
		return err
	}

	if err := helpers.ConfigureAuto(); err != nil {
		return fmt.Errorf("framework configuration failed: %w", err)
	}

	if opts.IncludeSample {
		if err := helpers.RunMagentoSampleData(); err != nil {
			return err
		}
	}

	return nil
}

// PreConfigure is Magento 2/Mage-OS's PreConfigureHook: it just generates
// app/etc/env.php before `govard config auto` runs. No variant
// parameterization needed - env.php generation (crypt key, DB connection
// block, table prefix) is identical for both distributions, driven
// entirely by the caller-provided closure (which itself resolves the
// right local DB credentials and probes the remote independently of which
// Magento distribution this is).
func PreConfigure(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return helpers.EnsureMagentoEnvPHP()
}

// PostClone is Magento 2/Mage-OS's PostCloneHook: create the admin user
// (if requested), then reindex. Moved verbatim from internal/cmd/
// bootstrap_remote.go's runBootstrapRemote (the
// `if opts.AdminCreate && engine.IsMagento2Family(...)` /
// `if engine.IsMagento2Family(...)` pair near the end of that function),
// same order, same error propagation (admin-create failures are
// swallowed by RunMagentoAdminCreate itself and never reach here; a
// reindex failure does propagate).
func PostClone(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	if opts.AdminCreate {
		if err := helpers.RunMagentoAdminCreate(); err != nil {
			return err
		}
	}
	return helpers.RunMagentoReindex()
}
