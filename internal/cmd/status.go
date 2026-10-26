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
	Short: "List the current project and every running Govard environment",
	Long:  "Workspace-wide environment overview: the current directory's project first (Running or Down), then every running environment. Use `govard env ps` for the current project's containers.",
	Run: func(cmd *cobra.Command, args []string) {
		running, err := engine.GetRunningProjectNames(cmd.Context())
		if err != nil {
			pterm.Error.Println(err)
			return
		}

		current := ""
		if config, err := loadFullConfig(); err == nil {
			current = config.ProjectName
		}
		entries, _ := engine.ReadProjectRegistryEntries()
		data := statusTableData(current, running, entries)
		if data == nil {
			pterm.Info.Println("No running Govard environments found.")
			return
		}

		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Environments ")
		fmt.Println()
		for i := 1; i < len(data); i++ {
			data[i][0] = pterm.Magenta(data[i][0])
			if data[i][1] == "Running" {
				data[i][1] = pterm.Green(data[i][1])
			} else {
				data[i][1] = pterm.Gray(data[i][1])
			}
			data[i][3] = pterm.Cyan(data[i][3])
		}
		_ = pterm.DefaultTable.WithHasHeader().WithData(data).Render()
	},
}

// statusTableData lists the current project first (Running or Down), then
// every other running project. It returns nil when there is nothing to show.
func statusTableData(current string, running []string, entries []engine.ProjectRegistryEntry) pterm.TableData {
	isRunning := map[string]bool{}
	for _, name := range running {
		isRunning[name] = true
	}
	var names []string
	if current != "" {
		names = append(names, current)
	}
	for _, name := range running {
		if name != current {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	data := pterm.TableData{{"Project", "Status", "Domain", "Profile"}}
	for _, project := range names {
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
		state := "Down"
		if isRunning[project] {
			state = "Running"
		}
		data = append(data, []string{project, state, domain, profile})
	}
	return data
}

// StatusTableDataForTest exposes statusTableData.
func StatusTableDataForTest(current string, running []string, entries []engine.ProjectRegistryEntry) pterm.TableData {
	return statusTableData(current, running, entries)
}
