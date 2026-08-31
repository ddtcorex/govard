package magento2

import (
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/types"

	"github.com/spf13/cobra"
)

const BinMagento = "bin/magento"

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:                   "magento2",
		Aliases:                []string{"magento", "m2"},
		DisplayName:            "Magento 2",
		MigrationTypes:         types.MigrationTypes{DDEV: []string{"magento2"}, Warden: []string{"magento2"}},
		Config:                 config,
		FrontendSyncDiscoverer: FrontendSyncDiscoverer,
		FrontendSyncRenderer:   FrontendSyncRenderer,
		Manifest:               Manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultMagentoDBUser,
			Password: conventions.DefaultMagentoDBPass,
			Database: conventions.DefaultMagentoDBName,
		},
		DefaultChownDirectories: []string{conventions.DefaultWorkDir, conventions.HomeWWWData + "/.cache/composer"},
		PHPStanPaths:            []string{"app/code", "app/design"},
		AuditLint: &types.AuditLintProfile{
			ProjectPHPVersions:    []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"},
			StandalonePHPVersions: []string{"8.1", "8.2", "8.3", "8.4", "8.5"},
			Linters:               []string{"phpcs", "phpstan"},
			CodingStandard:        "Magento2",
			PHPStanLevel:          5,
			PHPStanExtension:      "bitexpert/phpstan-magento",
		},
		AuditProfiler: &types.AuditProfilerProfile{
			EnvironmentVariable: "MAGE_PROFILER",
			EnvironmentValue:    "csvfile",
			OutputPath:          "var/log/profiler.csv",
		},
		AuditTargetResolver:    ResolveAuditTarget,
		ComposerCodingStandard: types.ComposerCodingStandard{Package: "magento/magento-coding-standard", Standard: "Magento2"},
		ComposerAuth:           types.ComposerAuthRequirement{Repository: "repo.magento.com", DisplayName: "Magento 2", CredentialURL: "https://marketplace.magento.com/customer/accessKeys/"},
		ToolCommands: []types.ToolCommand{
			{Name: "magento", Short: "Run Magento CLI commands", Binary: "php", PrependArgs: []string{"bin/magento"}},
			{Name: "magerun", Aliases: []string{"mr"}, Short: "Run n98-magerun commands", Binary: "n98-magerun"},
		},
		TestSuiteCommands: map[string]types.TestCommand{
			"mftf":        {Label: "MFTF Tests", Binary: "php", Args: []string{"vendor/bin/mftf", "run:group"}},
			"integration": {Label: "Magento 2 Integration Tests", Binary: "php", Args: []string{"-c", "dev/tests/integration/phpunit.xml", "vendor/bin/phpunit"}},
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"magento/product-community-edition", "magento/product-enterprise-edition", "magento/framework"},
			AuthJSONHosts:    []string{"repo.magento.com"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &Magento2Manager{}
		},
		FreshInstall:                freshInstall,
		PreConfigureHook:            PreConfigure,
		PostCloneHook:               PostClone,
		FreshInstallNeedsDomain:     true,
		SupportsBootstrap:           true,
		SupportsFreshInstall:        true,
		MinimumBootstrapVersion:     "2.0.0",
		DefaultFreshMetaPackage:     "magento/project-community-edition",
		PHPImageVariant:             "magento2",
		DBDriverCategory:            "magento",
		Upgrade:                     Upgrade,
		RunMappingAssetPreparer:     PrepareRunMappingAssets,
		TablePrefixDetector:         DetectTablePrefix,
		ResolveBootstrapTablePrefix: ResolveBootstrapTablePrefix,
		BuildDeployLocalesQuery:     BuildDeployLocalesQuery,
		BootstrapPlanSteps:          BootstrapPlanSteps,
		EnableVarnishOnInit:         true,
		VersionProfileResolver:      ResolveVersionProfile,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeMagento2Environment(remoteName, remoteCfg)
			return metadata.DB, err
		},
		ProbeRemoteBootstrapMetadata: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeMagento2Environment(remoteName, remoteCfg)
			if err != nil {
				return remote.RemoteDatabaseMetadata{}, err
			}
			metadata.DB.Private = map[string]string{"crypt_key": metadata.CryptKey}
			return metadata.DB, nil
		},
		RemoteDBUsesConfigTablePrefix: true,
		ResolveRemoteAdminPath:        DetectRemoteAdminPath,
		DetectLocalAdminMetadata:      DetectLocalAdminMetadata,
		BuildLocalAdminSettingsQuery:  BuildLocalAdminSettingsQuery,
		ResolveLocalAdminURL:          ResolveLocalAdminURL,
		ConfigureAfterProfileShift: func(config engine.Config, shift *engine.ProfileShiftInfo) error {
			return ConfigureMagento(config.ProjectName, config, false, shift)
		},
		PostSync: func(config engine.Config) error {
			return FixProjectPermissions(config.ProjectName, config)
		},
		UnblockSearchIndex: func(config engine.Config) error {
			return FixElasticsearchIndexBlock(config.ProjectName, config)
		},
		BuildSearchHostFixSQL: func(config engine.Config) string {
			host := "elasticsearch"
			if configured := strings.ToLower(strings.TrimSpace(config.Stack.Services.Search)); configured != "" && configured != "none" {
				host = configured
			}
			return BuildMagentoSearchHostFixSQL(host, ResolveMagentoSearchEngine(config))
		},
		BootstrapEnvironmentPath:        "app/etc/env.php",
		BootstrapEnvironmentMetadataKey: "crypt_key",
		RenderBootstrapEnvironment: func(secret string, database types.BootstrapEnvironmentDatabase, tablePrefix string) string {
			return BuildBootstrapEnvironment(secret, BootstrapEnvironmentDatabase{
				Database: database.Database,
				Username: database.Username,
				Password: database.Password,
			}, tablePrefix)
		},
		AutoConfigure: func(cmd *cobra.Command, config engine.Config) error {
			return ConfigureMagento(config.ProjectName, config, true, nil)
		},
	}
}
