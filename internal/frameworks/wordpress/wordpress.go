package wordpress

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/shared/dotenv"
	"govard/internal/frameworks/types"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:           "wordpress",
		Aliases:        []string{"wp"},
		DisplayName:    "WordPress",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"wordpress"}, Warden: []string{"wordpress"}},
		Config:         config,
		Manifest:       manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultWordPressDBUser,
			Password: conventions.DefaultWordPressDBPass,
			Database: conventions.DefaultWordPressDBName,
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"johnpbloch/wordpress", "roots/wordpress", "wordpress/wordpress"},
		},
		ComposerCodingStandard: types.ComposerCodingStandard{Package: "wp-coding-standards/wpcs", Standard: "WordPress"},
		ToolCommands: []types.ToolCommand{
			{Name: "wp", Short: "Run WordPress CLI commands", Binary: "wp"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewWordPressBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &WordPressManager{}
		},
		FreshInstall:                            freshInstall,
		FreshInstallNeedsDB:                     true,
		FreshInstallNeedsDomain:                 true,
		SupportsBootstrap:                       true,
		SupportsFreshInstall:                    true,
		PrepareComposer:                         FixWordPressCompatibility,
		RequiresComposerManifestForDumpAutoload: true,
		PostEnvironmentUp:                       FixWordPressCompatibility,
		IgnorePostCloneError: func(err error, projectDir string) bool {
			if err == nil {
				return false
			}
			_, statErr := os.Stat(filepath.Join(projectDir, "wp-config.php"))
			return statErr == nil
		},
		DBDriverCategory: "wordpress",
		Upgrade:          Upgrade,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeEnvironment(remoteName, remoteCfg)
			if err != nil {
				// Bedrock-style WordPress sites keep DB creds in .env, not wp-config.php -
				// fall back to the generic dotenv probe, matching the pre-existing
				// two-step behavior in resolveRemoteDBCredentials.
				dotenv, dotenvErr := dotenv.ProbeEnvironment(remoteName, remoteCfg)
				if dotenvErr == nil {
					return remote.RemoteDatabaseMetadata{
						Host:     dotenv.DB.Host,
						Port:     dotenv.DB.Port,
						Username: dotenv.DB.Username,
						Password: dotenv.DB.Password,
						Database: dotenv.DB.Database,
					}, nil
				}
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
		AutoConfigure: func(cmd *cobra.Command, config engine.Config) error {
			return nil
		},
	}
}
