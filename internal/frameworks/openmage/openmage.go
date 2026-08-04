package openmage

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/types"

	"github.com/spf13/cobra"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "openmage",
		DisplayName: "OpenMage",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultOpenMageDBUser,
			Password: conventions.DefaultOpenMageDBPass,
			Database: conventions.DefaultOpenMageDBName,
		},
		// Detect is intentionally the zero value - OpenMage has no
		// detection heuristic of its own. A project using
		// openmage/magento-lts is auto-detected as "magento1", not
		// "openmage" (see internal/frameworks/magento1/magento1.go's
		// Detect.ComposerPackages comment). Pre-existing behavior,
		// unchanged by this migration.
		Detect: engine.DetectionSpec{},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewOpenMageBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &magento1.Magento1Manager{}
		},
		FreshInstall:            freshInstall,
		FreshInstallNeedsDB:     true,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
		PHPImageVariant:         "magento1",
		DBDriverCategory:        "openmage",
		RunMappingAssetPreparer: magento2.PrepareRunMappingAssets,
		TablePrefixDetector:     magento1.DetectTablePrefix,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := magento1.ProbeMagento1Environment(remoteName, remoteCfg)
			return metadata.DB, err
		},
		AutoConfigure: func(cmd *cobra.Command, config engine.Config) error {
			return magento2.ConfigureMagento1(config.ProjectName, config)
		},
	}
}

// Spec declares OpenMage as a Magento 1 child. OpenMage keeps its deliberate
// lack of auto-detection and its own fresh-install/runtime deltas while
// inheriting common Magento 1 behavior.
func Spec() types.FrameworkSpec {
	def := Definition()
	return types.FrameworkSpec{
		Parent: "magento1",
		Definition: types.FrameworkDefinition{
			Name:    def.Name,
			Aliases: def.Aliases,
		},
		Patch: types.FrameworkPatch{
			DisplayName:             types.Set(def.DisplayName),
			MigrationTypes:          types.Clear[types.MigrationTypes](),
			Config:                  types.Set(def.Config),
			DefaultDBCredentials:    types.Set(def.DefaultDBCredentials),
			Detect:                  types.Clear[engine.DetectionSpec](),
			Bootstrap:               types.Set(def.Bootstrap),
			FreshInstall:            types.Set(def.FreshInstall),
			FreshInstallNeedsDB:     types.Set(def.FreshInstallNeedsDB),
			FreshInstallNeedsDomain: types.Set(def.FreshInstallNeedsDomain),
			DBDriverCategory:        types.Set(def.DBDriverCategory),
			Upgrade:                 types.Clear[engine.UpgradeFunc](),
		},
	}
}
