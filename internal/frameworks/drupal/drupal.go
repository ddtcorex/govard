package drupal

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/frameworks/shared/dotenv"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:           "drupal",
		DisplayName:    "Drupal",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"drupal7", "drupal8", "drupal9", "drupal10", "drupal11"}},
		Config:         config,
		Manifest:       manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"drupal/core"},
		},
		ComposerCodingStandard: types.ComposerCodingStandard{Package: "drupal/coder", Standard: "Drupal"},
		ToolCommands: []types.ToolCommand{
			{Name: "drush", Short: "Run Drupal Drush commands", Binary: "drush"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewDrupalBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
		DBDriverCategory:     "drupal",
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
