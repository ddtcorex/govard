package cmd

import (
	"errors"
	"fmt"
	"strings"

	"govard/internal/cli"
	"govard/internal/engine"
	"govard/internal/ui"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var projectCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:     "project",
	Aliases: []string{"prj", "projects", "registry"},
	Short:   "Browse known projects from registry",
}

var projectOpenCmd = &cobra.Command{
	Annotations: map[string]string{
		// Resolves a registry entry and prints its path; nothing here starts or
		// inspects a container.
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "open <query>",
	Short: "Find a project by fuzzy query and print its path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectOpen(cmd, args[0])
	},
}

var projectListShowOrphans bool

var projectListCmd = &cobra.Command{
	Annotations: map[string]string{
		// The listing reads the Govard registry only. The Docker-resource scan
		// that used to ride on --orphans now lives in `project orphans`, which
		// keeps the group's requirement.
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all available projects in the registry",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectList(cmd)
	},
}

var projectOrphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Show Docker resources that are not in the registry",
	Long: `List compose projects that exist in Docker but are absent from the Govard
registry. This inspects the container runtime, so unlike 'project list' it needs
Docker.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectOrphans(cmd)
	},
}

var projectDeleteForce bool

var projectDeleteCmd = &cobra.Command{
	Use:     "delete <query>",
	Aliases: []string{"rm", "del", "remove"},
	Short:   "Completely remove a project and its Docker resources",
	Long: `Permanently delete a project from Govard.
This command will:
1. Stop all containers for the project.
2. Remove all Docker volumes (database data, etc.).
3. Unregister project domains from proxy and hosts.
4. Remove the project from the Govard registry.

WARNING: This action is destructive and cannot be undone (for volumes).
It does NOT delete your project source code.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectDelete(cmd, args[0])
	},
}

func initProjectCommands() {
	projectCmd.AddCommand(projectOpenCmd)

	projectListCmd.Flags().BoolVar(&projectListShowOrphans, "orphans", false, "Show projects that have Docker resources but are not in the registry")
	// Kept hidden and refused: the scan moved to `project orphans` so that
	// `project list` itself needs no container runtime. An old invocation gets
	// the new command instead of "unknown flag".
	_ = projectListCmd.Flags().MarkHidden("orphans")
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectOrphansCmd)

	projectDeleteCmd.Flags().BoolVarP(&projectDeleteForce, "force", "f", false, "Delete without confirmation")
	projectCmd.AddCommand(projectDeleteCmd)
}

func runProjectOpen(cmd *cobra.Command, query string) error {
	match, _, err := engine.FindProjectByQuery(query)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), match.Path)
	return nil
}

func runProjectDelete(cmd *cobra.Command, query string) error {
	match, score, err := engine.FindProjectByQuery(query)

	// If we have no registry match OR a weak registry match,
	// check if there's an EXACT match in the orphaned projects.
	if err != nil || score >= engine.ScoreAmbiguousThreshold {
		orphans, orphanErr := engine.GetOrphanedComposeProjects(cmd.Context())
		if orphanErr == nil {
			for _, o := range orphans {
				if strings.EqualFold(o.Name, query) {
					return runOrphanDelete(cmd, o)
				}
			}
		}
	}

	if err != nil {
		return err
	}

	// For destructive operations, we only allow strong matches (exact, prefix, or substring).
	// If the match is weak (subsequence etc.), we require it to be forced or we error out.
	if score >= engine.ScoreAmbiguousThreshold && !projectDeleteForce {
		pterm.Warning.Printf("Weak match for %q: project %s (score: %d)\n", query, match.ProjectName, score)
		pterm.Warning.Println("For your safety, Govard requires a stronger match (prefix, exact, or path) for deletion.")
		pterm.Warning.Println("Please use a more specific name or the full project path.")
		return fmt.Errorf("match for %q is too weak for destructive operation", query)
	}

	if !projectDeleteForce {
		pterm.Warning.Printf("You are about to delete project: %s\n", match.ProjectName)
		pterm.Warning.Println("This will remove all Docker containers and VOLUMES (database data).")
		pterm.Warning.Printf("Project path: %s\n", match.Path)
		fmt.Println()

		result, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show("Are you sure you want to proceed?")
		if !result {
			pterm.Info.Println("Deletion cancelled.")
			return nil
		}
	}

	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Printf(" DELETING PROJECT: %s \n", match.ProjectName)
	fmt.Println()

	spinner, _ := pterm.DefaultSpinner.Start("Cleaning up resources...")
	err = engine.DeleteProject(cmd.Context(), match.Path, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error))
	if err != nil {
		spinner.Fail(err.Error())
		return err
	}
	spinner.Success("Project deleted successfully.")

	return nil
}

func runOrphanDelete(cmd *cobra.Command, orphan engine.OrphanProject) error {
	if !projectDeleteForce {
		pterm.Warning.Printf("You are about to delete an UNREGISTERED project: %s\n", orphan.Name)
		pterm.Warning.Println("This project was found in Docker but is not in the Govard registry.")
		pterm.Warning.Println("This will remove all Docker containers and VOLUMES (database data).")
		fmt.Println()

		result, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show("Are you sure you want to proceed?")
		if !result {
			pterm.Info.Println("Deletion cancelled.")
			return nil
		}
	}

	fmt.Println()
	pterm.NewStyle(pterm.BgLightRed, pterm.FgWhite, pterm.Bold).Printf(" DELETING ORPHAN PROJECT: %s \n", orphan.Name)
	fmt.Println()

	spinner, _ := pterm.DefaultSpinner.Start("Cleaning up orphaned resources...")
	err := engine.DeleteOrphanProject(cmd.Context(), orphan.Name, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error))
	if err != nil {
		spinner.Fail(err.Error())
		return err
	}
	spinner.Success("Orphaned project resources removed.")

	return nil
}

func runProjectList(cmd *cobra.Command) error {
	if projectListShowOrphans {
		return &cli.UsageError{Err: errors.New("project list --orphans moved to `govard project orphans`")}
	}

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		pterm.Info.Println("The project registry is empty. Initialize projects with 'govard init'.")
		return nil
	}

	tableData := [][]string{
		{"Project", "Framework", "Domain", "Path"},
	}

	for _, entry := range entries {
		framework := entry.Framework
		if framework == "" {
			framework = "unknown"
		}
		tableData = append(tableData, []string{
			entry.ProjectName,
			framework,
			entry.Domain,
			entry.Path,
		})
	}

	err = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	if err != nil {
		return err
	}

	return nil
}

// runProjectOrphans reports compose projects that exist in Docker but not in the
// Govard registry. It inspects the container runtime, which is why it is its own
// command instead of a flag on the registry-only listing.
func runProjectOrphans(cmd *cobra.Command) error {
	orphans, err := engine.GetOrphanedComposeProjects(cmd.Context())
	if err != nil {
		return err
	}

	if len(orphans) == 0 {
		pterm.Info.Println("No orphaned Docker resources: every compose project is in the registry.")
		return nil
	}

	pterm.NewStyle(pterm.BgLightYellow, pterm.FgBlack, pterm.Bold).Println(" ORPHANED PROJECTS (IN DOCKER BUT NOT REGISTRY) ")
	fmt.Println()

	orphanData := [][]string{
		{"Project", "Status", "ConfigFiles"},
	}
	for _, o := range orphans {
		orphanData = append(orphanData, []string{o.Name, o.Status, o.ConfigFiles})
	}
	return pterm.DefaultTable.WithHasHeader().WithData(orphanData).Render()
}
