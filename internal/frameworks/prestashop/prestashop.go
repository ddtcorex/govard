package prestashop

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/frameworks/types"
	"os"
	"path/filepath"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "prestashop",
		DisplayName: "PrestaShop",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultPrestaShopDBUser,
			Password: conventions.DefaultPrestaShopDBPass,
			Database: conventions.DefaultPrestaShopDBName,
		},
		Detect: engine.DetectionSpec{
			FilePaths: []string{"config/defines.inc.php"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "prestashop", Short: "Run PrestaShop CLI commands (Symfony console)", Binary: "php", PrependArgs: []string{"bin/console"}},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewPrestaShopBootstrap(opts)
		},
		SupportsBootstrap:   true,
		DBDriverCategory:    "prestashop",
		TablePrefixDetector: DetectTablePrefix,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeEnvironment(remoteName, remoteCfg)
			return metadata.DB, err
		},
		ProbeRemoteBootstrapMetadata: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeEnvironment(remoteName, remoteCfg)
			if err != nil {
				return remote.RemoteDatabaseMetadata{}, err
			}
			metadata.DB.Private = metadata.Secrets.metadata()
			return metadata.DB, nil
		},
		RemoteDBUsesConfigTablePrefix: true,
		IgnorePostCloneError: func(err error, projectDir string) bool {
			if err == nil {
				return false
			}
			_, statErr := os.Stat(filepath.Join(projectDir, "app", "config", "parameters.php"))
			return statErr == nil
		},
	}
}
