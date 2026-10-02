package magento2

import (
	"fmt"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
)

// FamilyVariant parameterizes the shared Magento-2-family fresh-install/
// clone-workflow pipeline for a specific distribution - the same pattern
// this package's own upgrade.go's UpgradeVariant already uses for `govard
// upgrade`. Magento 2 owns this type and the generic logic
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
//
// When opts.PHPVersion is known and the runtime runs Composer 2, the project
// is created without installing, its composer.json gets
// config.platform.php = opts.PHPVersion, and dependencies are installed
// against that platform. Extension and library requirements stay ignored as
// before, but the php requirement is enforced, so the resolved vendor tree
// parses on the container's PHP. Without a PHP version, or on a Composer
// older than 2.2 (Composer 1 has no --ignore-platform-req=<name> option and
// 2.0/2.1 have no ext-* wildcard, while --ignore-platform-reqs would make the
// pin inert), the command is the original single create-project.
func BuildFreshCreateProjectCommand(variant FamilyVariant, opts bootstrap.Options) string {
	versionPart := ""
	if opts.Version != "" {
		versionPart = " " + engine.ShellQuote(opts.Version)
	}
	const stagingDir = "/tmp/govard-create-project"
	syncStep := "if command -v rsync >/dev/null 2>&1; then rsync -a " + stagingDir + "/ " + conventions.DefaultWorkDir + "/; else cp -a " + stagingDir + "/. " + conventions.DefaultWorkDir + "/; fi"

	if !platformPinApplies(opts.PHPVersion, opts.ComposerVersion) {
		return strings.Join([]string{
			"set -e",
			"rm -rf " + stagingDir,
			"composer create-project -n --ignore-platform-reqs --repository-url=" + variant.RepositoryURL + " " +
				engine.ShellQuote(opts.MetaPackage) + " " + stagingDir + versionPart,
			syncStep,
			"rm -rf " + stagingDir,
		}, " && ")
	}

	return strings.Join([]string{
		"set -e",
		"rm -rf " + stagingDir,
		// --add-repository persists the repository into the new
		// composer.json, so the separate install below resolves from it too.
		"composer create-project -n --no-install --ignore-platform-reqs --add-repository --repository-url=" + variant.RepositoryURL + " " +
			engine.ShellQuote(opts.MetaPackage) + " " + stagingDir + versionPart,
		"composer --working-dir=" + stagingDir + " config platform.php " + engine.ShellQuote(opts.PHPVersion),
		"composer --working-dir=" + stagingDir + " install -n --ignore-platform-req='ext-*' --ignore-platform-req='lib-*'",
		syncStep,
		"rm -rf " + stagingDir,
	}, " && ")
}

// platformPinApplies reports whether a composer platform.php pin can be
// written and honored: the PHP version is known and the Composer pin selects
// 2.2 or later. "" and "latest" mean the newest Composer, and "2" downloads
// the newest 2.x; a pin below 2.2, or one that does not parse, keeps the
// unpinned commands.
func platformPinApplies(phpVersion, composerVersion string) bool {
	if strings.TrimSpace(phpVersion) == "" {
		return false
	}
	composerVersion = strings.TrimSpace(composerVersion)
	switch composerVersion {
	case "", "latest", "2":
		return true
	}
	return engine.IsNumericDotVersionAtLeast(composerVersion, "2.2")
}

// magentoOpenSearchEngineMinVersion is the first Magento line whose
// setup:install and catalog/search/engine take the "opensearch" value for an
// OpenSearch backend. Adobe: "For versions earlier than 2.4.6, use the
// elasticsearch7 value for the Elasticsearch 7 or OpenSearch engine. For
// version 2.4.6 and later, use the opensearch value for the OpenSearch
// engine."
// https://experienceleague.adobe.com/en/docs/commerce-operations/upgrade-guide/prepare/prerequisites
const magentoOpenSearchEngineMinVersion = "2.4.6"

// magentoSearchInstallOptionsMinVersion is the first Magento line whose
// setup:install accepts --search-engine and --elasticsearch-*. Measured
// 2026-10-02 against the magento2 repository: setup/src/Magento/Setup/Model/
// SearchConfigOptionsList.php (which declares them) is 404 at tags 2.3.5,
// 2.3.7, 2.3.7-p4 and 2.4.0-beta1 and 200 from 2.4.0. Earlier lines (2.3
// searches with mysql by default) must receive no search arguments, or
// setup:install rejects them as unknown options.
const magentoSearchInstallOptionsMinVersion = "2.4.0"

// BuildSetupInstallArgs builds the `bin/magento setup:install` argument
// list for variant - moved verbatim from internal/cmd/
// bootstrap_post_install.go's runBootstrapPostInstall (the "build
// setupArgs" half; the "run it" half is now
// internal/cmd/bootstrap_fresh_install.go's generic setup-install runner,
// wired through CmdHelpers.RunFrameworkSetupInstall).
//
// The elasticsearch7-vs-opensearch version gate is intentionally
// magento2-only (variant.Name == "magento2") - Mage-OS versions always get
// OpenSearch regardless of version, which is a known, pre-existing
// asymmetry. Magento lines before 2.4.6 get elasticsearch7, 2.4.6 and later
// get opensearch (see magentoOpenSearchEngineMinVersion).
func BuildSetupInstallArgs(variant FamilyVariant, version string, adminEmail string, tablePrefix string) []string {
	searchEngine := conventions.ServiceOpenSearch
	withSearch := true
	if variant.Name == "magento2" && version != "" {
		if comparison, comparable := engine.CompareNumericDotVersions(version, magentoOpenSearchEngineMinVersion); comparable && comparison < 0 {
			searchEngine = "elasticsearch7"
		}
		// Before 2.4.0 setup:install has no search options at all.
		if comparison, comparable := engine.CompareNumericDotVersions(version, magentoSearchInstallOptionsMinVersion); comparable && comparison < 0 {
			withSearch = false
		}
	}

	setupArgs := []string{
		"setup:install",
		"--backend-frontname=" + conventions.DefaultAdminPath,
		"--db-host=" + conventions.DefaultMagentoDBHost,
		"--db-name=" + variant.DBName,
		"--db-user=" + variant.DBUser,
		"--db-password=" + variant.DBPass,
		"--db-prefix=" + tablePrefix,
	}
	if withSearch {
		setupArgs = append(setupArgs, setupInstallSearchArgs(searchEngine)...)
	}
	return append(setupArgs,
		"--admin-user="+conventions.DefaultAdminUser,
		"--admin-password="+conventions.DefaultAdminPassword,
		"--admin-firstname=Admin",
		"--admin-lastname=User",
		"--admin-email="+adminEmail,
	)
}

// setupInstallSearchArgs returns the setup:install search block for a search
// engine name: "opensearch" uses the --opensearch-* options, any other name
// (elasticsearch7) the --elasticsearch-* options. Both point at the stack's
// "elasticsearch" service host.
func setupInstallSearchArgs(searchEngine string) []string {
	prefix := "elasticsearch"
	if searchEngine == conventions.ServiceOpenSearch {
		prefix = conventions.ServiceOpenSearch
	}
	return []string{
		"--search-engine=" + searchEngine,
		"--" + prefix + "-host=elasticsearch",
		"--" + prefix + "-port=9200",
		"--" + prefix + "-index-prefix=magento2",
		"--" + prefix + "-enable-auth=0",
		"--" + prefix + "-timeout=15",
	}
}

// FreshInstall runs the shared Magento 2/Mage-OS fresh-install sequence:
// EnsureAuthJSON -> FixComposerCompatibility -> create-project -> (if
// HyvaInstall) Hyva theme install -> setup:install -> `govard config auto`
// -> (if IncludeSample) sample data. Moved from internal/cmd/
// bootstrap_fresh_install.go's runBootstrapFreshInstall, parameterized by
// variant instead of a core framework-name check.
func FreshInstall(variant FamilyVariant, opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	if err := helpers.EnsureAuthJSON(); err != nil {
		return err
	}
	if err := helpers.FixComposerCompatibility(); err != nil {
		return err
	}

	commandLine := BuildFreshCreateProjectCommand(variant, opts)
	if err := opts.Runner(commandLine); err != nil {
		if platformPinApplies(opts.PHPVersion, opts.ComposerVersion) {
			return fmt.Errorf("fresh create-project failed: %w (composer resolved dependencies for PHP %s, the project's stack.php_version; if this %s version needs another PHP, run `govard config set stack.php_version <version>` and retry)", err, opts.PHPVersion, variant.DisplayName)
		}
		return fmt.Errorf("fresh create-project failed: %w", err)
	}

	if opts.HyvaInstall {
		if err := helpers.RunHyvaInstall(); err != nil {
			return err
		}
	}

	tablePrefix, err := helpers.ResolveFrameworkTablePrefix()
	if err != nil {
		return err
	}
	adminEmail := conventions.AdminEmailForDomain(opts.Domain)
	setupArgs := BuildSetupInstallArgs(variant, opts.Version, adminEmail, tablePrefix)
	if err := RunSetupInstall(helpers, setupArgs); err != nil {
		return err
	}

	if err := helpers.ConfigureAuto(); err != nil {
		return fmt.Errorf("framework configuration failed: %w", err)
	}

	if opts.IncludeSample {
		if err := RunSampleData(helpers); err != nil {
			return err
		}
	}

	return nil
}

func RunSetupInstall(helpers bootstrap.CmdHelpers, args []string) error {
	if helpers.IsPHPContainerRunning != nil && helpers.IsPHPContainerRunning() {
		fix := []string{"exec", "-T", "php", "sh", "-c", "curl -s -X PUT 'http://elasticsearch:9200/_all/_settings' -H 'Content-Type: application/json' -d'{\"index.blocks.read_only_allow_delete\": null}' > /dev/null 2>&1 || true"}
		if err := helpers.RunEnvironmentCommand(fix); err != nil {
			fmt.Printf("Warning: failed to apply Elasticsearch block fix: %v\n", err)
		}
	}
	if err := helpers.RunTool("magento", args); err != nil {
		return fmt.Errorf("magento setup:install failed: %w", err)
	}
	return nil
}

// RunSampleData executes Magento's ordered sample-data workflow through the
// generic tool transport owned by bootstrap.CmdHelpers.
func RunSampleData(helpers bootstrap.CmdHelpers) error {
	for _, args := range [][]string{{"sample:deploy"}, {"setup:upgrade"}, {"indexer:reindex"}, {"cache:flush"}} {
		if err := helpers.RunTool("magento", args); err != nil {
			return fmt.Errorf("sample data step failed (%s): %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

func BootstrapPlanSteps(createAdmin bool) []types.BootstrapPlanStep {
	steps := []types.BootstrapPlanStep{{Description: "Configuring framework environment...", Command: "govard config auto"}}
	if createAdmin {
		steps = append(steps, types.BootstrapPlanStep{Description: "Creating framework admin user...", Command: "govard tool magento admin:user:create ..."})
	}
	return append(steps, types.BootstrapPlanStep{Description: "Reindexing framework data...", Command: "govard tool magento indexer:reindex"})
}

// PreConfigure is Magento 2/Mage-OS's PreConfigureHook: it just generates
// app/etc/env.php before `govard config auto` runs. No variant
// parameterization needed - env.php generation (crypt key, DB connection
// block, table prefix) is identical for both distributions, driven
// entirely by the caller-provided closure (which itself resolves the
// right local DB credentials and probes the remote independently of which
// Magento distribution this is).
func PreConfigure(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	return helpers.EnsureFrameworkEnvironment()
}

// PostClone is Magento 2/Mage-OS's PostCloneHook: create the admin user
// (if requested), then reindex. Moved verbatim from internal/cmd/
// bootstrap_remote.go's runBootstrapRemote (the
// former core family conditional near the end of that function),
// same order, same error propagation (admin-create failures are
// swallowed by RunAdminCreate itself and never reach here; a reindex
// failure does propagate).
func PostClone(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	if opts.AdminCreate {
		RunAdminCreate(opts, helpers)
	}
	return RunReindex(helpers)
}

func RunReindex(helpers bootstrap.CmdHelpers) error {
	pterm.Info.Println("Reindexing Magento data...")
	if helpers.IsPHPContainerRunning != nil && !helpers.IsPHPContainerRunning() {
		return nil
	}
	return helpers.RunTool("magento", []string{"indexer:reindex"})
}

func RunAdminCreate(opts bootstrap.Options, helpers bootstrap.CmdHelpers) {
	if helpers.IsPHPContainerRunning != nil && !helpers.IsPHPContainerRunning() {
		return
	}
	if helpers.RunToolSilent != nil {
		_ = helpers.RunToolSilent("magento", []string{"admin:user:create", "--admin-user=" + conventions.DefaultAdminUser, "--admin-password=" + conventions.DefaultAdminPassword, "--admin-firstname=Govard", "--admin-lastname=Admin", "--admin-email=" + conventions.AdminEmailForDomain(opts.Domain)})
	}
}
