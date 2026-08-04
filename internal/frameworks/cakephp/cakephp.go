package cakephp

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
		Name:        "cakephp",
		DisplayName: "CakePHP",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"cakephp/cakephp"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "cake", Short: "Run CakePHP CLI commands", Binary: "bin/cake"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewCakePHPBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
		DBDriverCategory:     "cakephp",
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
