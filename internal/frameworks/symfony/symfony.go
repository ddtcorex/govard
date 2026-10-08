package symfony

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
	DefaultDBUser = "symfony"
	DefaultDBPass = "symfony"
	DefaultDBName = "symfony"
	BinConsole    = "bin/console"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:           "symfony",
		DisplayName:    "Symfony",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"symfony"}, Warden: []string{"symfony"}},
		Config:         config,
		Manifest:       manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: DefaultDBUser,
			Password: DefaultDBPass,
			Database: DefaultDBName,
		},
		AuditLint: &types.AuditLintProfile{
			ProjectPHPVersions:    []string{"8.1", "8.2", "8.3", "8.4"},
			StandalonePHPVersions: []string{"8.1", "8.2", "8.3", "8.4"},
			Linters:               []string{"phpcs", "phpstan"},
			CodingStandard:        "Symfony",
			PHPStanLevel:          5,
			PHPStanExtension:      "phpstan/phpstan-symfony",
			RuleIDPrefix:          "SYMFONY-LINT",
		},
		AuditTargetResolver: ResolveAuditTarget,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"symfony/framework-bundle", "symfony/symfony"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "symfony", Short: "Run Symfony CLI commands", Binary: "php", PrependArgs: []string{"bin/console"}},
		},
		// Only commands the Symfony skeleton guarantees: an item for an optional
		// bundle (doctrine-migrations) would be permanently red, the defect class
		// P3-13/P3-14 already suffer from.
		VerifyToolItems: []engine.VerifyToolItem{
			{ID: "P3-SYM-01", Phase: 3, Title: "govard tool symfony --version", Tool: "symfony", Args: []string{"--version"}},
			{ID: "P3-SYM-02", Phase: 3, Title: "govard tool symfony cache:clear", Tool: "symfony", Args: []string{"cache:clear"}},
			{ID: "P3-SYM-03", Phase: 3, Title: "govard tool symfony debug:router", Tool: "symfony", Args: []string{"debug:router"}},
			{ID: "P5-SYM-01", Phase: 5, Title: "govard tool symfony cache:clear after restore", Tool: "symfony", Args: []string{"cache:clear"}},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewSymfonyBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &SymfonyManager{}
		},
		FreshInstall:         freshInstall,
		FreshInstallNeedsDB:  true,
		SupportsBootstrap:    true,
		SupportsFreshInstall: true,
		DBDriverCategory:     "symfony",
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
