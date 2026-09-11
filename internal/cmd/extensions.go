package cmd

import (
	"os"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var extensionForce bool

var extensionsCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:     "extensions",
	Aliases: []string{"ext"},
	Short:   "Manage project extension contract in .govard",
}

var extensionsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create .govard extension scaffolding for the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		changed, err := engine.EnsureExtensionContract(wd, extensionForce)
		if err != nil {
			return err
		}

		if len(changed) == 0 {
			pterm.Info.Println("Extension contract already exists. No files changed.")
			return nil
		}

		pterm.Success.Println("Extension contract scaffolded:")
		for _, file := range changed {
			pterm.Println(" - " + file)
		}
		return nil
	},
}

func init() {
	extensionsInitCmd.Flags().BoolVar(&extensionForce, "force", false, "Overwrite existing extension scaffold files")
	extensionsCmd.AddCommand(extensionsInitCmd)
}
