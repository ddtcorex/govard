package cmd

import (
	"fmt"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var statusCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
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
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Environments ")
		fmt.Println()

		tableData := pterm.TableData{
			{"Project", "Status", "Domain", "Profile"},
		}

		for _, project := range running {
			domain := project + ".test"
			profile := "-"
			for _, entry := range entries {
				if entry.ProjectName == project {
					if entry.Domain != "" {
						domain = entry.Domain
					}
					if entry.Profile != "" {
						profile = entry.Profile
					}
					break
				}
			}

			tableData = append(tableData, []string{
				pterm.Magenta(project),
				pterm.Green("Running"),
				domain,
				pterm.Cyan(profile),
			})
		}

		_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	},
}
