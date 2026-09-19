package cmd

import (
	"os"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var (
	upgradeCmdVersion       string
	upgradeCmdDryRun        bool
	upgradeCmdNoDB          bool
	upgradeCmdNoEnv         bool
	upgradeCmdNoInteraction bool
)

var upgradeCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "upgrade",
	Short: "Upgrade the framework version",
	Long:  "Upgrade the framework version of the current project. The target comes from the required --version flag.",
	Example: `  govard upgrade --version 2.4.8-p4
  govard upgrade --version 11 --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		if config.Framework == "" {
			pterm.Warning.Println("No framework configured in .govard.yml.")
			return nil
		}

		cwd, _ := os.Getwd()
		opts := engine.UpgradeOptions{
			TargetVersion: upgradeCmdVersion,
			DryRun:        upgradeCmdDryRun,
			NoDBUpgrade:   upgradeCmdNoDB,
			NoEnvUpdate:   upgradeCmdNoEnv,
			NoInteraction: upgradeCmdNoInteraction,
			Stdout:        cmd.OutOrStdout(),
			Stderr:        cmd.ErrOrStderr(),
			ProjectDir:    cwd,
			ProjectName:   config.ProjectName,
		}
		return engine.UpgradeFramework(cmd.Context(), config, opts)
	},
}

func init() {
	upgradeCmd.Flags().StringVar(&upgradeCmdVersion, "version", "", "Target version, required (e.g. 2.4.8-p4)")
	upgradeCmd.Flags().BoolVar(&upgradeCmdDryRun, "dry-run", false, "Print steps without executing them")
	upgradeCmd.Flags().BoolVar(&upgradeCmdNoDB, "no-db-upgrade", false, "Skip database setup:upgrade step (Magento 2 only)")
	upgradeCmd.Flags().BoolVar(&upgradeCmdNoEnv, "no-env-update", false, "Skip profile application and container restart")
	upgradeCmd.Flags().BoolVarP(&upgradeCmdNoInteraction, "yes", "y", false, "Do not ask for confirmation")
	rootCmd.AddCommand(upgradeCmd)
}

// UpgradeCommand exposes the upgrade command for tests.
func UpgradeCommand() *cobra.Command {
	return upgradeCmd
}
