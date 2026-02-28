package cmd

import (
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List running Govard project environments across the workspace",
	Long:  "Workspace-wide environment overview. Use `govard env ps` for the current project only.",
	Run: func(cmd *cobra.Command, args []string) {
		running, err := engine.GetRunningProjectNames(cmd.Context())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		if len(running) == 0 {
			pterm.Info.Println("No running Govard environments found.")
			return
		}

		entries, _ := engine.ReadProjectRegistryEntries()
		pterm.DefaultHeader.WithFullWidth().Println("Govard Environments")

		tableData := pterm.TableData{
			{"Project", "Status", "Domain"},
		}

		for _, project := range running {
			domain := project + ".test"
			for _, entry := range entries {
				if entry.ProjectName == project && entry.Domain != "" {
					domain = entry.Domain
					break
				}
			}

			tableData = append(tableData, []string{
				pterm.Magenta(project),
				pterm.Green("Running"),
				domain,
			})
		}

		_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	},
}
