package mageos

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/types"

	"github.com/spf13/cobra"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "mageos",
		DisplayName: "Mage-OS",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultMageOSDBUser,
			Password: conventions.DefaultMageOSDBPass,
			Database: conventions.DefaultMageOSDBName,
		},
		PHPStanPaths:        []string{"app/code", "app/design"},
		AuditTargetResolver: ResolveAuditTarget,
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{
				"mage-os/product-community-edition",
				"mage-os/project-community-edition",
			},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewBootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &magento2.Magento2Manager{}
		},
		FreshInstall:            freshInstall,
		PreConfigureHook:        magento2.PreConfigure,
		PostCloneHook:           magento2.PostClone,
		FreshInstallNeedsDomain: true,
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
		PHPImageVariant:         "magento2",
		Upgrade:                 Upgrade,
		RunMappingAssetPreparer: magento2.PrepareRunMappingAssets,
		TablePrefixDetector:     magento2.DetectTablePrefix,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := magento2.ProbeMagento2Environment(remoteName, remoteCfg)
			return metadata.DB, err
		},
		AutoConfigure: func(cmd *cobra.Command, config engine.Config) error {
			return magento2.ConfigureMagento(config.ProjectName, config, true, nil)
		},
	}
}

// Spec declares Mage-OS as a Magento 2 child. Every inherited behavior is
// intentionally omitted; only distribution-specific deltas remain here.
func Spec() types.FrameworkSpec {
	def := Definition()
	return types.FrameworkSpec{
		Parent: "magento2",
		Definition: types.FrameworkDefinition{
			Name:    def.Name,
			Aliases: def.Aliases,
		},
		Patch: types.FrameworkPatch{
			DisplayName:              types.Set(def.DisplayName),
			MigrationTypes:           types.Clear[types.MigrationTypes](),
			Config:                   types.Set(def.Config),
			DefaultDBCredentials:     types.Set(def.DefaultDBCredentials),
			ComposerCodingStandard:   types.Clear[types.ComposerCodingStandard](),
			ComposerAuth:             types.Clear[types.ComposerAuthRequirement](),
			Detect:                   types.Set(def.Detect),
			AuditTargetResolver:      types.Set(def.AuditTargetResolver),
			Bootstrap:                types.Set(def.Bootstrap),
			FreshInstall:             types.Set(def.FreshInstall),
			DefaultFreshMetaPackage:  types.Set("mage-os/project-community-edition"),
			DBDriverCategory:         types.Clear[string](),
			Upgrade:                  types.Set(def.Upgrade),
			VersionProfileResolver:   types.Clear[engine.VersionProfileResolver](),
			VarnishTemplateFramework: types.Set("magento2"),
		},
	}
}
