package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"strings"
)

// MagentoFamilyVariant parameterizes the shared Magento-2-family
// fresh-install pipeline for a specific distribution (Magento 2 or
// Mage-OS) - the same pattern internal/engine/upgrade_magento2.go's
// magentoUpgradeVariant already uses for `govard upgrade`.
type MagentoFamilyVariant struct {
	Name          string
	DisplayName   string
	RepositoryURL string
	DBName        string
	DBUser        string
	DBPass        string
}

var Magento2Variant = MagentoFamilyVariant{
	Name:          "magento2",
	DisplayName:   "Magento 2",
	RepositoryURL: "https://repo.magento.com",
	DBName:        conventions.DefaultMagentoDBName,
	DBUser:        conventions.DefaultMagentoDBUser,
	DBPass:        conventions.DefaultMagentoDBPass,
}

var MageOSVariant = MagentoFamilyVariant{
	Name:          "mageos",
	DisplayName:   "Mage-OS",
	RepositoryURL: "https://repo.mage-os.org",
	DBName:        conventions.DefaultMageOSDBName,
	DBUser:        conventions.DefaultMageOSDBUser,
	DBPass:        conventions.DefaultMageOSDBPass,
}

// magento2FamilyBootstrap adapts Magento2FreshCommands/MageOSFreshCommands
// (plain functions, unlike every other framework's FrameworkBootstrap
// struct) to the FrameworkBootstrap interface, so the registry's Bootstrap
// factory field can be populated for magento2/mageos too. Only
// FreshCommands is real: Magento 2 and Mage-OS's actual fresh-install
// orchestration is MagentoFamilyFreshInstall (below), reached via their
// own Definition().FreshInstall, not this generic interface, so the other
// lifecycle methods report that explicitly instead of silently no-op'ing.
type magento2FamilyBootstrap struct {
	options       Options
	name          string
	freshCommands func(Options) []string
}

func NewMagento2Bootstrap(opts Options) *magento2FamilyBootstrap {
	return &magento2FamilyBootstrap{options: opts, name: "magento2", freshCommands: Magento2FreshCommands}
}

func NewMageOSBootstrap(opts Options) *magento2FamilyBootstrap {
	return &magento2FamilyBootstrap{options: opts, name: "mageos", freshCommands: MageOSFreshCommands}
}

func (m *magento2FamilyBootstrap) Name() string {
	return m.name
}

func (m *magento2FamilyBootstrap) SupportsFreshInstall() bool {
	return true
}

func (m *magento2FamilyBootstrap) SupportsClone() bool {
	return true
}

func (m *magento2FamilyBootstrap) FreshCommands() []string {
	return m.freshCommands(m.options)
}

func (m *magento2FamilyBootstrap) unsupported(step string) error {
	return fmt.Errorf("%s %s is orchestrated by 'govard bootstrap --fresh' (internal/frameworks/%s/freshinstall.go), not bootstrap.FrameworkBootstrap", m.name, step, m.name)
}

func (m *magento2FamilyBootstrap) CreateProject(projectDir string) error {
	return m.unsupported("project creation")
}

func (m *magento2FamilyBootstrap) Install(projectDir string) error {
	return m.unsupported("installation")
}

func (m *magento2FamilyBootstrap) Configure(projectDir string) error {
	return m.unsupported("configuration")
}

func (m *magento2FamilyBootstrap) PostClone(projectDir string) error {
	return m.unsupported("post-clone setup")
}

// BuildMagentoFreshCreateProjectCommand builds the shell command for a
// fresh composer create-project against variant's repository - moved
// verbatim from internal/cmd/bootstrap_fresh_install.go's
// bootstrapFreshCreateProjectCommandLine, parameterized by variant instead
// of a framework-name string check.
func BuildMagentoFreshCreateProjectCommand(variant MagentoFamilyVariant, opts Options) string {
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

// BuildMagentoSetupInstallArgs builds the `bin/magento setup:install`
// argument list for variant - moved verbatim from internal/cmd/
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
func BuildMagentoSetupInstallArgs(variant MagentoFamilyVariant, version string, adminEmail string, tablePrefix string) []string {
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

// MagentoFamilyFreshInstall runs the shared Magento 2/Mage-OS fresh-install
// sequence: EnsureAuthJSON -> FixComposerCompatibility -> create-project ->
// (if HyvaInstall) Hyva theme install -> setup:install -> `govard config
// auto` -> (if IncludeSample) sample data. Moved from
// internal/cmd/bootstrap_fresh_install.go's runBootstrapFreshInstall,
// parameterized by variant instead of an engine.Magento2FamilyDisplayName
// string check.
func MagentoFamilyFreshInstall(variant MagentoFamilyVariant, opts Options, projectDir string, helpers CmdHelpers) error {
	if err := helpers.EnsureAuthJSON(); err != nil {
		return err
	}
	if err := helpers.FixComposerCompatibility(); err != nil {
		return err
	}

	commandLine := BuildMagentoFreshCreateProjectCommand(variant, opts)
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
	setupArgs := BuildMagentoSetupInstallArgs(variant, opts.Version, adminEmail, tablePrefix)
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
