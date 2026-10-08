package laravel

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/shared/dotenv"
	"govard/internal/frameworks/types"
)

const (
	DefaultDBUser = "laravel"
	DefaultDBPass = "laravel"
	DefaultDBName = "laravel"
	BinArtisan    = "artisan"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:           "laravel",
		DisplayName:    "Laravel",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"laravel"}, Warden: []string{"laravel"}},
		Config:         config,
		Manifest:       manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: DefaultDBUser,
			Password: DefaultDBPass,
			Database: DefaultDBName,
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"laravel/framework"},
		},
		AuditLint: &types.AuditLintProfile{
			ProjectPHPVersions:    []string{"8.1", "8.2", "8.3", "8.4"},
			StandalonePHPVersions: []string{"8.1", "8.2", "8.3", "8.4"},
			Linters:               []string{"phpcs", "phpstan"},
			CodingStandard:        "PSR12",
			PHPStanLevel:          5,
			RuleIDPrefix:          "LARAVEL-LINT",
		},
		AuditTargetResolver:    ResolveAuditTarget,
		ComposerCodingStandard: types.ComposerCodingStandard{Package: "laravel/pint", Standard: "Laravel"},
		ToolCommands: []types.ToolCommand{
			{Name: "artisan", Short: "Run Laravel Artisan commands", Binary: "php", PrependArgs: []string{"artisan"}},
		},
		// The framework's own `govard verify` items. Declared here rather than
		// branched on in internal/verify, so a Laravel project gets a dev-loop
		// checklist instead of the 10 Magento-gated items silently vanishing.
		VerifyToolItems: []engine.VerifyToolItem{
			{ID: "P3-LAR-01", Phase: 3, Title: "govard tool artisan --version", Tool: "artisan", Args: []string{"--version"}},
			{ID: "P3-LAR-02", Phase: 3, Title: "govard tool artisan migrate:status", Tool: "artisan", Args: []string{"migrate:status"}},
			{ID: "P3-LAR-03", Phase: 3, Title: "govard tool artisan cache:clear", Tool: "artisan", Args: []string{"cache:clear"}},
			{ID: "P5-LAR-01", Phase: 5, Title: "govard tool artisan migrate:status after restore", Tool: "artisan", Args: []string{"migrate:status"}},
		},
		DefaultTestCommand: types.TestCommand{Binary: "php", Args: []string{"artisan", "test"}},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewLaravelBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &LaravelManager{}
		},
		FreshInstall:         freshInstall,
		FreshInstallNeedsDB:  true,
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
		DBDriverCategory:     "laravel",
		Upgrade:              Upgrade,
		DeployRecipe:         DeployRecipe,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := dotenv.ProbeEnvironment(remoteName, remoteCfg)
			if err != nil {
				return remote.RemoteDatabaseMetadata{}, err
			}
			return remote.RemoteDatabaseMetadata{
				Host:     metadata.DB.Host,
				Port:     metadata.DB.Port,
				Username: metadata.DB.Username,
				Password: metadata.DB.Password,
				Database: metadata.DB.Database,
			}, nil
		},
	}
}
