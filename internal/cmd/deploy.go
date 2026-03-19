package cmd

import (
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the application",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := loadFullConfig()
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		if err := engine.RunHooks(config, engine.HookPreDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			pterm.Error.Printf("Pre-deploy hooks failed: %v\n", err)
			return
		}

		pterm.Info.Println("Deploying (strategy: native)")

		if err := engine.RunHooks(config, engine.HookPostDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			pterm.Error.Printf("Post-deploy hooks failed: %v\n", err)
			return
		}
	},
}

func init() {
	deployCmd.Flags().String("strategy", "native", "Deployment strategy (native or deployer)")
	deployCmd.Flags().Bool("deployer", false, "Use Deployer strategy")
	deployCmd.Flags().String("deployer-config", "", "Path to Deployer config")
	deployCmd.Flags().StringP("locales", "l", "", "Space-separated locales to deploy (e.g. \"en_US fr_FR\")")

	rootCmd.AddCommand(deployCmd)
}

// DeployCommand exposes the deploy command for tests.
func DeployCommand() *cobra.Command {
	return deployCmd
}
