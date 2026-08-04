package magento1

import (
	"fmt"

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
		Name:           "magento1",
		Aliases:        []string{"m1"},
		DisplayName:    "Magento 1",
		MigrationTypes: types.MigrationTypes{DDEV: []string{"magento"}, Warden: []string{"magento1"}},
		Config:         config,
		Manifest:       Manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultMagentoDBUser,
			Password: conventions.DefaultMagentoDBPass,
			Database: conventions.DefaultMagentoDBName,
		},
		// ComposerPackages intentionally includes openmage/magento-lts and
		// magento-hackathon/magento-composer-installer - this is the exact,
		// pre-existing behavior of internal/engine/discovery.go (a project
		// using openmage/magento-lts is auto-detected as "magento1", not
		// "openmage"; openmage has no detection heuristic of its own).
		// This looks like it could be a bug, but changing it is out of
		// scope - Global Constraints require zero detection behavior change.
		ToolCommands: []types.ToolCommand{
			{Name: "magerun", Aliases: []string{"mr"}, Short: "Run n98-magerun commands", Binary: "n98-magerun"},
		},
		Detect: engine.DetectionSpec{
			ComposerPackages: []string{"openmage/magento-lts", "magento-hackathon/magento-composer-installer"},
			FilePaths:        []string{"app/Mage.php", "app/etc/local.xml"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewMagento1Bootstrap(opts)
		},
		BaseURLManager: func() tunnel.BaseURLManager {
			return &Magento1Manager{}
		},
		// FreshInstall is unsupported by design for Magento 1 - Magento 1
		// itself has no create-project/setup:install equivalent, so
		// `govard bootstrap --fresh` just points the user at OpenMage
		// instead. SupportsFreshInstall stays true so this bespoke error
		// fires (via runBootstrapRegistryFreshInstall) instead of the
		// generic "framework doesn't support --fresh" rejection at the
		// CLI allowlist stage.
		FreshInstall: func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
			return fmt.Errorf("fresh install not supported for magento1 (use openmage instead)")
		},
		SupportsBootstrap:       true,
		SupportsFreshInstall:    true,
		PHPImageVariant:         "magento1",
		DBDriverCategory:        "magento",
		RunMappingAssetPreparer: magento2.PrepareRunMappingAssets,
		TablePrefixDetector:     DetectTablePrefix,
		BootstrapPlanSteps: func(bool) []types.BootstrapPlanStep {
			return []types.BootstrapPlanStep{{Description: "Configuring framework environment...", Command: "govard config auto"}}
		},
		Upgrade: Upgrade,
		ProbeRemoteDB: func(remoteName string, remoteCfg engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error) {
			metadata, err := ProbeMagento1Environment(remoteName, remoteCfg)
			return metadata.DB, err
		},
		RemoteDBUsesConfigTablePrefix: true,
		AutoConfigure: func(cmd *cobra.Command, config engine.Config) error {
			return magento2.ConfigureMagento1(config.ProjectName, config)
		},
	}
}
