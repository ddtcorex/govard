package types

import (
	"text/template"

	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"

	"github.com/spf13/cobra"
)

// BootstrapFactory builds a framework's bootstrapper for one invocation.
// A factory (not a pre-built instance) because bootstrap.Options carries
// per-invocation state (target version, DB creds, etc.) that must not be
// baked into a long-lived registry entry.
type BootstrapFactory func(bootstrap.Options) bootstrap.FrameworkBootstrap

// FrontendSyncRuntime is the framework-neutral identity returned by a
// frontend-sync provider. Mode remains a string so lifecycle orchestration
// does not need to import a framework package to select its checks.
type FrontendSyncRuntime struct {
	Mode           string
	Services       []string
	PublicEndpoint FrontendSyncPublicEndpoint
	HTMLInjection  *FrontendSyncHTMLInjection
}

// FrontendSyncPublicEndpoint describes the public path exposed by a frontend
// runtime. The command layer forwards this framework-owned declaration to the
// proxy without branching on framework or mode names.
type FrontendSyncPublicEndpoint struct {
	Path        string
	StripPrefix string
	Service     string
	Port        int
}

// FrontendSyncHTMLInjection identifies a runtime service that proxies the
// application and injects a development client into HTML responses.
type FrontendSyncHTMLInjection struct {
	Service string
	Port    int
}

// FrontendSyncDiscoverer inspects a project root and returns its
// project-owned frontend development runtime, if any. Commands obtain this
// capability through the framework registry instead of branching on
// framework names.
type FrontendSyncDiscoverer func(root string) (FrontendSyncRuntime, error)

// FrontendSyncRenderer renders a framework's dedicated frontend Compose
// blueprint and returns the resulting runtime plus the path to the rendered
// compose file.
type FrontendSyncRenderer func(root string, config engine.Config) (FrontendSyncRuntime, string, error)

// FrameworkDefinition is the single source of truth for one framework's
// identity, runtime defaults, sync/manifest data, and dispatch (bootstrap,
// base-URL rewriting, bootstrap-command support, fresh-install
// orchestration, clone-workflow hooks). Every registered framework now
// has FreshInstall populated except Magento 1 (fresh install unsupported
// by design - CreateProject just returns an error telling the user to
// use --clone) and PrestaShop (fresh install never supported, no
// FreshInstall field at all). Clone-workflow orchestration in
// internal/cmd/bootstrap_remote.go dispatches through the registry too -
// the generic FrameworkBootstrap.PostClone(projectDir) interface method
// for most frameworks, plus the two optional hook fields below
// (PreConfigureHook/PostCloneHook) for frameworks whose clone-workflow
// needs step timing or cmd-package capabilities (running `govard tool
// <x>`) that plain interface method can't express - Magento 2/Mage-OS
// are the first consumers, not the only intended ones.
type FrameworkDefinition struct {
	// Name is the canonical framework key, e.g. "magento2", "laravel".
	Name string
	// Parent is the canonical framework key this definition inherits from.
	// It is empty for root frameworks and is populated only on resolved child
	// definitions.
	Parent string
	// Aliases are additional strings that should resolve to Name (e.g.
	// "magento" -> "magento2").
	Aliases []string
	// DisplayName is a human-readable label, e.g. "Magento 2".
	DisplayName string
	// MigrationTypes lists external tool framework identifiers that should map
	// to this definition during DDEV or Warden configuration migration.
	MigrationTypes MigrationTypes

	// Config carries runtime/compose defaults (PHP version, includes list,
	// nginx template, etc.), currently sourced from engine.GetFrameworkConfig.
	Config engine.FrameworkConfig
	// FrontendSyncDiscoverer and FrontendSyncRenderer are optional because
	// most frameworks do not expose a project-owned frontend runtime - both
	// are nil for those. Mage-OS inherits Magento 2's functions.
	FrontendSyncDiscoverer FrontendSyncDiscoverer
	FrontendSyncRenderer   FrontendSyncRenderer
	// Manifest carries sync/media exclude and sensitive-table data,
	// currently sourced from engine.GetFrameworkManifestConfig.
	Manifest engine.FrameworkManifestConfig

	// DefaultDBCredentials holds the local-development database port/username/
	// password/database-name defaults for this framework. Host, Engine, and
	// TablePrefix are deliberately excluded - Host/TablePrefix are resolved at
	// runtime (container inspection, remote probing, user config), and Engine
	// is already derived from Config.DefaultDB via dbEngineForFramework, not a
	// literal per framework. Replaces the switch in
	// internal/cmd/db_credentials.go's defaultDBCredentialsForFrameworkFields.
	DefaultDBCredentials DefaultDBCredentials
	// DefaultChownDirectories lists additional paths requiring ownership repair
	// for this framework. Core always supplies its generic baseline path.
	DefaultChownDirectories []string

	// PHPStanPaths lists the default PHPStan analysis paths for this framework
	// (used both by `govard test phpstan` and `govard vscode setup`'s
	// phpstan.options fallback when no path args/project config are given),
	// nil for frameworks that use the generic {"app", "src"} default.
	PHPStanPaths []string

	// AuditLint declares exact lint policy for frameworks that support the
	// generic audit runner. Nil means lint audit is unsupported.
	AuditLint *AuditLintProfile
	// AuditTargetResolver resolves a framework-specific path into an audit
	// target. Nil means lint audit target selection is unsupported.
	AuditTargetResolver AuditTargetResolver
	// AuditProfiler declares a framework-owned stock runtime profiler. Nil
	// means the profiler audit check is unsupported.
	AuditProfiler *AuditProfilerProfile

	// ComposerCodingStandard is the Composer package (e.g.
	// "magento/magento-coding-standard") that registers this framework's phpcs
	// coding standard, and the --standard label to pass phpcs when that package
	// is required. Zero value (empty Package) means this framework has no
	// dedicated coding-standard package - replaces one entry of
	// internal/cmd/vscode_setup.go's composerCodingStandardPackages, which was
	// keyed by package name rather than framework.
	ComposerCodingStandard ComposerCodingStandard
	// ComposerAuth describes optional Composer repository credentials that this
	// framework requires for bootstrap dependency installation.
	ComposerAuth ComposerAuthRequirement

	// ToolCommands are framework-owned `govard tool` command declarations.
	// Their availability is derived from the resolved registry lineage rather
	// than a second framework-name allowlist in internal/cmd.
	ToolCommands []ToolCommand
	// DefaultTestCommand replaces the generic PHPUnit default when non-empty.
	DefaultTestCommand TestCommand
	// TestSuiteCommands contains explicitly supported named suites such as MFTF
	// or integration. Core only executes the resolved command.
	TestSuiteCommands map[string]TestCommand

	// Detect describes how to auto-detect this framework from a project
	// directory (composer.json/package.json/auth.json/file-path matches).
	// Populated by each framework's Definition() and pushed into
	// engine's detection registry by Registry.Register.
	Detect engine.DetectionSpec

	// Bootstrap builds this framework's fresh-install/clone bootstrapper.
	// Populated for all 14 frameworks; frameworks.RunBootstrap uses it to
	// dispatch without a per-framework switch.
	Bootstrap BootstrapFactory

	// BaseURLManager builds this framework's tunnel base-URL rewriter (for
	// `govard tunnel`). Nil for frameworks that don't need specialized
	// rewriting; frameworks.NewBaseURLManager falls back to
	// tunnel.NoopManager in that case.
	BaseURLManager func() tunnel.BaseURLManager

	// SupportsBootstrap allows `govard bootstrap` (remote/clone workflow)
	// for this framework.
	SupportsBootstrap bool
	// SupportsFreshInstall allows `govard bootstrap --fresh` for this
	// framework.
	SupportsFreshInstall bool
	// MinimumBootstrapVersion is the lowest numeric framework version accepted
	// by fresh-install bootstrap; empty means any valid numeric version.
	MinimumBootstrapVersion string
	// DefaultFreshMetaPackage is the framework's Composer package used when the
	// CLI still carries its generic default package value.
	DefaultFreshMetaPackage string
	// PrepareComposer performs framework-specific compatibility work before a
	// clone workflow installs dependencies. Generic bootstrap orchestration does
	// not need to know why the work is required.
	PrepareComposer func(config engine.Config) error
	// RequiresComposerManifestForDumpAutoload prevents a best-effort composer
	// dump-autoload call when composer.json is absent. Most frameworks leave it
	// false because their clone workflow can still have a usable vendor tree.
	RequiresComposerManifestForDumpAutoload bool

	// FreshInstall runs this framework's fresh-install orchestration
	// (CreateProject/Install/Configure sequencing, env-up timing, etc.),
	// replacing a per-framework case in
	// internal/cmd/bootstrap_fresh_install.go's switch. nil for
	// frameworks not yet migrated to this field - they keep dispatching
	// through that switch.
	FreshInstall func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
	// FreshInstallNeedsDB/FreshInstallNeedsDomain tell the caller which
	// bootstrap.Options fields (DB credentials, domain) to populate
	// before invoking FreshInstall. Only meaningful when FreshInstall is
	// non-nil.
	FreshInstallNeedsDB     bool
	FreshInstallNeedsDomain bool
	// FreshInstallManagesOwnEnvUp reports whether this framework's
	// FreshInstall already calls `env up` itself (and, if needed, runs
	// Install()/migrate against the running containers), so the generic
	// post-fresh-install `env up` in bootstrapCmd.RunE would be redundant.
	// Django's compose "web" container executes `python manage.py
	// runserver` directly and can't come up against an empty project
	// directory, so its fresh-install path must scaffold the project
	// first, then bring the environment up itself before running
	// Install() - by the time control returns to RunE, env up has
	// already happened.
	FreshInstallManagesOwnEnvUp bool

	// PreConfigureHook runs framework-specific setup that must happen
	// during the remote/clone bootstrap workflow, before `govard config
	// auto` - for frameworks whose configure step depends on a generated
	// file existing first (Magento 2/Mage-OS's app/etc/env.php, generated
	// from a template plus any probed remote crypt key/table prefix).
	// Optional; nil for frameworks that don't need it. Runs unconditionally
	// (not gated on opts.ComposerInstall) whenever set, since env.php
	// generation is a prerequisite for config auto rather than a
	// consequence of composer install.
	PreConfigureHook func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
	// PostCloneHook runs additional framework-specific steps during the
	// remote/clone bootstrap workflow, after the generic
	// def.Bootstrap(opts).PostClone(projectDir) dispatch (which some
	// frameworks - e.g. Magento 2/Mage-OS - don't implement at all,
	// reporting it as unsupported since their real post-clone setup is
	// this hook instead). Optional; nil for frameworks that don't need
	// it. The caller gates this on opts.ComposerInstall alone, NOT on
	// shouldRunFrameworkPostClone/FrameworkSupportsPostClone - those also
	// check engine.FrameworkSupportsPostClone, which is deliberately false
	// for frameworks that only use this hook (their plain PostClone really
	// is unsupported), so gating on that combined condition would make
	// this hook permanently unreachable for exactly the frameworks that set it.
	PostCloneHook func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
	// IgnorePostCloneError allows a framework to recognize an already-complete
	// local configuration after its clone hook reports an otherwise non-fatal
	// error. Core retains generic composer/vendor fallback handling.
	IgnorePostCloneError func(err error, projectDir string) bool

	// PHPImageVariant is the Docker image variant suffix (e.g. "magento1",
	// "magento2") this framework's PHP container needs instead of the plain
	// "php" image - "" for frameworks that use the plain image. engine can't
	// import this package (frameworks imports engine, not the reverse), so
	// frameworks.Register pushes this into engine.RegisterPHPImageVariant
	// instead of engine reading the field directly; see
	// internal/engine/runtime_images.go's PHPImageVariantForFramework.
	PHPImageVariant string
	// NodeImageFlavor controls the Node image tag shape for this framework.
	// "standard" selects node:<version>; empty uses the generic alpine image.
	NodeImageFlavor string
	// VarnishTemplateFramework optionally identifies another framework whose
	// Varnish blueprint assets this definition intentionally reuses.
	VarnishTemplateFramework string

	// DBDriverCategory is the phpMyAdmin per-project DB-user/label category
	// this framework's database credentials use (e.g. "magento" for both
	// magento1 and magento2), injected into the generated PHP config for the
	// global phpMyAdmin container. "" means this framework has no category of
	// its own - the generated PHP falls back to "app". engine can't import this
	// package (frameworks imports engine, not the reverse), so
	// frameworks.Register pushes this into engine.RegisterDBDriverCategory
	// instead of engine reading the field directly; see
	// internal/engine/proxy.go's DBDriverCategoryForFramework.
	DBDriverCategory string

	// Upgrade runs this framework's upgrade pipeline (dependency bump,
	// migrations, cache flush, etc.), replacing a per-framework case in
	// internal/engine/upgrade.go's UpgradeFramework switch. Populated by
	// frameworks.Register via engine.RegisterUpgrader. nil for frameworks
	// with no upgrade pipeline implemented yet - UpgradeFramework reports
	// "not implemented" for them, matching pre-existing behavior.
	Upgrade engine.UpgradeFunc
	// RunMappingAssetPreparer prepares this framework's per-store
	// nginx/apache "run mapping" assets before a blueprint renders (e.g.
	// Magento's mage-run-map.conf files), replacing internal/engine's
	// former prepareMagentoRunMappingAssets/isMagentoFramework gate.
	// Populated by frameworks.Register via
	// engine.RegisterRunMappingAssetPreparer. nil for frameworks with
	// nothing to prepare here (10 of 14).
	RunMappingAssetPreparer engine.RunMappingAssetPreparer

	// TablePrefixDetector reads this framework's own config file (env.php,
	// local.xml, parameters.php, etc.) and returns its configured database
	// table prefix, replacing a per-framework case in
	// internal/engine/table_prefix.go's FrameworkSupportsTablePrefix/
	// DetectMagentoTablePrefix switches. Populated by frameworks.Register via
	// engine.RegisterTablePrefixDetector. nil for frameworks with no
	// table-prefix concept (most of the 14) - FrameworkSupportsTablePrefix
	// reports false for them, matching pre-existing behavior.
	TablePrefixDetector         engine.TablePrefixDetector
	ResolveBootstrapTablePrefix func(configuredPrefix string) (string, error)
	BuildDeployLocalesQuery     func(tablePrefix string) string
	BootstrapPlanSteps          func(createAdmin bool) []BootstrapPlanStep
	EnableVarnishOnInit         bool

	// VersionProfileResolver resolves this framework's version-specific
	// runtime-profile overrides (e.g. Magento 2's per-patch-release stack),
	// replacing the hardcoded `if framework == "magento2"` branch in
	// internal/engine/profile.go's ResolveRuntimeProfile. Populated by
	// frameworks.Register via engine.RegisterVersionProfileResolver. nil for
	// every framework except magento2 today - all others rely on the generic
	// JSON-driven resolveFrameworkProfileFromRegistry fallback instead.
	VersionProfileResolver engine.VersionProfileResolver

	// TemplateFuncs contributes additional blueprint-template functions this
	// framework needs (e.g. emdash's/nextjs's runtime-command builders),
	// merged into the FuncMap available to every rendered blueprint template.
	// Keyed the same way the template references them (e.g.
	// {{ emdashRuntimeCommand ... }}). nil for frameworks with no custom
	// template functions - 12 of the 14 today.
	TemplateFuncs template.FuncMap

	// ProbeRemoteDB probes a configured remote (SSH host + path) for this
	// framework's live database credentials, replacing a per-framework case
	// in internal/cmd/db_credentials.go's resolveRemoteDBCredentials switch.
	// Returns remote.RemoteDatabaseMetadata, the generic transport shape
	// (Host/Port/Username/Password/Database/TablePrefix). Frameworks whose own
	// probe returns a narrower type leave TablePrefix empty. nil for frameworks with
	// no remote-DB probing implemented (custom, django, nextjs, emdash) -
	// resolveRemoteDBCredentials falls back to remoteCfg.DBName/User/Pass/Port
	// the same way the switch's default/"custom" cases do today.
	ProbeRemoteDB func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error)
	// ProbeRemoteBootstrapMetadata retrieves remote metadata required by a
	// framework's post-clone bootstrap. Generic core forwards its opaque Private
	// values without knowing their framework-specific meaning.
	ProbeRemoteBootstrapMetadata func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error)
	// RemoteDBUsesConfigTablePrefix allows config.TablePrefix to act as a
	// fallback only for frameworks whose historical remote DB workflow has a
	// table-prefix concept. A probed prefix always wins. Frameworks such as
	// WordPress and dotenv-based applications leave this false so a local
	// config value cannot silently alter a remote import/query.
	RemoteDBUsesConfigTablePrefix bool

	// DefaultAdminPath is this framework's conventional admin route. Empty
	// uses the product-wide default. It keeps non-standard paths such as
	// Emdash's _emdash/admin with their framework definition instead of in
	// command or desktop callers.
	DefaultAdminPath string
	// ResolveRemoteAdminPath optionally probes a remote deployment for its
	// configured admin route. Core consumes the returned path without knowing
	// which framework's configuration file supplied it.
	ResolveRemoteAdminPath func(remoteName string, remoteCfg engine.RemoteConfig) (string, error)
	// DetectLocalAdminMetadata reads framework-owned local admin routing data.
	DetectLocalAdminMetadata func(projectRoot string) (frontName string, tablePrefix string)
	// BuildLocalAdminSettingsQuery returns the framework-owned query for local
	// admin URL settings; core only executes it against the local database.
	BuildLocalAdminSettingsQuery func(tablePrefix string) string
	// ResolveLocalAdminURL applies framework-specific local admin URL policy.
	ResolveLocalAdminURL func(baseURL string, envFrontName string, dbValues map[string]string) string
	// PostEnvironmentUp performs framework-specific compatibility work after
	// containers become ready. Generic lifecycle orchestration reports failures
	// but does not need to identify the framework.
	PostEnvironmentUp func(config engine.Config) error
	// ConfigureAfterProfileShift applies framework tuning after a detected
	// runtime-profile change. The caller controls whether and when to prompt;
	// the framework owns the resulting configuration work.
	ConfigureAfterProfileShift func(config engine.Config, shift *engine.ProfileShiftInfo) error
	// PostSync performs framework-specific filesystem repair after a sync that
	// transferred files or media.
	PostSync func(config engine.Config) error
	// UnblockSearchIndex clears a framework-specific search-engine write block
	// when `doctor --fix` identifies that condition.
	UnblockSearchIndex func(config engine.Config) error
	// BuildSearchHostFixSQL returns the framework-specific SQL required to
	// point a local installation at its configured search service.
	BuildSearchHostFixSQL func(config engine.Config) string
	// BootstrapEnvironmentPath identifies a framework-owned runtime file that
	// must be rendered before clone configuration. Empty means none is needed.
	BootstrapEnvironmentPath string
	// BootstrapEnvironmentMetadataKey identifies the opaque remote metadata
	// value used as the environment renderer's secret. Core forwards the value
	// without assigning framework meaning to it.
	BootstrapEnvironmentMetadataKey string
	// RenderBootstrapEnvironment renders a framework's bootstrap runtime file
	// from generic connection data and a framework-owned secret.
	RenderBootstrapEnvironment func(secret string, database BootstrapEnvironmentDatabase, tablePrefix string) string

	// AutoConfigure runs `govard config auto`'s framework-specific
	// post-render setup (env.php generation, table-prefix/crypt-key wiring,
	// etc.), replacing a per-framework case in
	// internal/cmd/config_auto.go's applyFrameworkAutoConfiguration switch.
	// nil for frameworks with nothing to do here - every unlisted framework
	// gets a "not supported yet" warning; wordpress explicitly registers a
	// no-op instead of leaving this nil, to distinguish "confirmed nothing to
	// do" from "not supported".
	AutoConfigure func(cmd *cobra.Command, config engine.Config) error
}

// DefaultDBCredentials is the port/username/password/database-name shape
// for FrameworkDefinition.DefaultDBCredentials. Mirrors the relevant subset
// of internal/cmd's dbCredentials struct (Host/Engine/TablePrefix excluded
// - see FrameworkDefinition.DefaultDBCredentials's doc comment for why).
type DefaultDBCredentials struct {
	Port     int
	Username string
	Password string
	Database string
}

type BootstrapPlanStep struct {
	Description string
	Command     string
}

// MigrationTypes is the set of external tool identifiers that correspond to a
// framework. The canonical name remains FrameworkDefinition.Name.
type MigrationTypes struct {
	DDEV   []string
	Warden []string
}

// BootstrapEnvironmentDatabase is the common local DB data supplied to a
// framework's bootstrap-environment renderer.
type BootstrapEnvironmentDatabase struct {
	Database string
	Username string
	Password string
}

// ComposerCodingStandard pairs a Composer package name with the phpcs
// --standard label it registers. See FrameworkDefinition.ComposerCodingStandard.
type ComposerCodingStandard struct {
	Package  string
	Standard string
}

// ComposerAuthRequirement defines the Composer repository and user-facing
// guidance for framework-owned dependency credentials. An empty Repository
// means no interactive authentication is required.
type ComposerAuthRequirement struct {
	Repository    string
	DisplayName   string
	CredentialURL string
}
