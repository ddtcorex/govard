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
		AuditTargetResolver: ResolveAuditTarget,
		ToolCommands: []types.ToolCommand{
			{Name: "artisan", Short: "Run Laravel Artisan commands", Binary: "php", PrependArgs: []string{"artisan"}},
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
