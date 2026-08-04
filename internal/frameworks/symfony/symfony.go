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

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:           "symfony",
		DisplayName:    "Symfony",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"symfony"}, Warden: []string{"symfony"}},
		Config:         config,
		Manifest:       manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultSymfonyDBUser,
			Password: conventions.DefaultSymfonyDBPass,
			Database: conventions.DefaultSymfonyDBName,
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"symfony/framework-bundle", "symfony/symfony"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "symfony", Short: "Run Symfony CLI commands", Binary: "php", PrependArgs: []string{"bin/console"}},
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
