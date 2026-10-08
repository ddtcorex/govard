package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"govard/internal/cli"
	"govard/internal/engine"
	"govard/internal/projectstores"
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
	Short:   "Manage known projects from the registry (list, open, orphans, delete)",
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
4. Remove the project's files under the Govard home (compose file and its
   .hash, varnish/rabbitmq/nginx/apache directories, its active-projects.json
   entry), its frontend containers and network, and its sandbox (container,
   database volume, image, key and mirror).
5. Remove the project from the Govard registry.

<query> is a project name, domain or path. A project that is down, or already
missing from the registry, is still found when it left files or Docker
resources behind. Docker resources are matched by exact compose project name.

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

// confirmDestructive asks a yes/no question for a destructive command. The
// prompt needs a terminal: with a piped or closed stdin it would wait forever,
// so a non-terminal stdin is a refusal that names the flag which skips the
// question, and nothing is removed.
func confirmDestructive(question, escapeFlag string) (bool, error) {
	if !stdinIsTerminal() {
		return false, fmt.Errorf("confirmation required but stdin is not a terminal; pass %s to proceed without prompting", escapeFlag)
	}
	result, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show(question)
	return result, nil
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
		// A project that is down or no longer in the registry can still have
		// left files and Docker resources behind. Resolve it by exact name or
		// by its directory instead of answering "no project matches".
		if art, ok := engine.DiscoverProjectByQuery(cmd.Context(), query); ok {
			return runUnregisteredDelete(cmd, art)
		}
	}

	if err != nil {
		return fmt.Errorf("%w (no leftover files or Docker resources were found for it either)", err)
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
		printProjectArtifactList(projectArtifactsForEntry(cmd, match))
		fmt.Println()

		result, confirmErr := confirmDestructive("Are you sure you want to proceed?", "--force")
		if confirmErr != nil {
			return confirmErr
		}
		if !result {
			pterm.Info.Println("Deletion cancelled.")
			return nil
		}
	}

	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Printf(" DELETING PROJECT: %s \n", match.ProjectName)
	fmt.Println()

	spinner, _ := pterm.DefaultSpinner.Start("Cleaning up resources...")
	cleanupProjectSandbox(cmd.Context(), match.ProjectName, match.Path, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error))
	err = engine.DeleteProject(cmd.Context(), match.Path, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error))
	if err != nil {
		spinner.Fail(err.Error())
		return err
	}
	spinner.Success("Project deleted successfully.")

	return nil
}

// runUnregisteredDelete removes a project that is not in the registry but left
// files or Docker resources behind (for example after `env down`, or after the
// registry entry was already dropped).
func runUnregisteredDelete(cmd *cobra.Command, art engine.ProjectArtifacts) error {
	if !projectDeleteForce {
		pterm.Warning.Printf("You are about to delete an UNREGISTERED project: %s\n", art.Name)
		pterm.Warning.Println("It is not in the Govard registry, but it left these behind:")
		printProjectArtifactList(art)
		pterm.Warning.Println("This will remove its Docker containers and VOLUMES (database data).")
		fmt.Println()

		result, confirmErr := confirmDestructive("Are you sure you want to proceed?", "--force")
		if confirmErr != nil {
			return confirmErr
		}
		if !result {
			pterm.Info.Println("Deletion cancelled.")
			return nil
		}
	}

	fmt.Println()
	pterm.NewStyle(pterm.BgLightRed, pterm.FgWhite, pterm.Bold).Printf(" DELETING UNREGISTERED PROJECT: %s \n", art.Name)
	fmt.Println()

	spinner, _ := pterm.DefaultSpinner.Start("Cleaning up leftover resources...")
	cleanupProjectSandbox(cmd.Context(), art.Name, art.Root, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error))
	if err := engine.DeleteProjectByName(cmd.Context(), art, ui.NewPtermWriter(&pterm.Info), ui.NewPtermWriter(&pterm.Error)); err != nil {
		spinner.Fail(err.Error())
		return err
	}
	spinner.Success("Project resources removed.")
	return nil
}

// projectArtifactsForEntry lists what deleting a registered project will
// remove beyond its containers, so the confirmation shows the whole blast
// radius.
func projectArtifactsForEntry(cmd *cobra.Command, entry engine.ProjectRegistryEntry) engine.ProjectArtifacts {
	art := engine.CollectProjectArtifacts(entry.ProjectName, entry.Path, []string{entry.Profile, entry.PreviousProfile})
	art.Docker = engine.ComposeProjectResourcesExist(cmd.Context(), entry.ProjectName)
	return art
}

func printProjectArtifactList(art engine.ProjectArtifacts) {
	for _, line := range art.Lines() {
		pterm.Warning.Printf("  - %s\n", line)
	}
}

func runOrphanDelete(cmd *cobra.Command, orphan engine.OrphanProject) error {
	if art := engine.CollectProjectArtifacts(orphan.Name, "", nil); engine.SafeProjectArtifactName(orphan.Name) {
		art.Docker = true
		return runUnregisteredDelete(cmd, art)
	}
	if !projectDeleteForce {
		pterm.Warning.Printf("You are about to delete an UNREGISTERED project: %s\n", orphan.Name)
		pterm.Warning.Println("This project was found in Docker but is not in the Govard registry.")
		pterm.Warning.Println("This will remove all Docker containers and VOLUMES (database data).")
		fmt.Println()

		result, confirmErr := confirmDestructive("Are you sure you want to proceed?", "--force")
		if confirmErr != nil {
			return confirmErr
		}
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

	// Status needs the container runtime, but the listing must keep working
	// without one: an unreachable Docker degrades to "unknown" and keeps the
	// registry order.
	var running map[string]bool
	if names, runErr := projectListRunningNames(cmd.Context()); runErr == nil {
		running = make(map[string]bool, len(names))
		for _, name := range names {
			running[name] = true
		}
	}
	rows := orderProjectListRows(entries, running)

	tableData := [][]string{
		{"Project", "Status", "Framework", "Domain", "Path"},
	}
	tableData = append(tableData, rows...)

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

func init() {
	// The verify-run, audit and lint-cache stores are keyed by ids derived from
	// the project path; internal/projectstores resolves them for `project delete`
	// (the desktop app registers the same resolver).
	projectstores.Register()
}

// projectListRunningNames reports the running Govard compose projects. It is a
// variable so tests can stand in for the Docker daemon.
var projectListRunningNames = engine.GetRunningProjectNames

// orderProjectListRows builds the table rows with running projects first. A nil
// running map means the runtime could not be queried: every status is
// "unknown" and the registry order is kept.
func orderProjectListRows(entries []engine.ProjectRegistryEntry, running map[string]bool) [][]string {
	ordered := append([]engine.ProjectRegistryEntry(nil), entries...)
	if running != nil {
		sort.SliceStable(ordered, func(i, j int) bool {
			return running[ordered[i].ProjectName] && !running[ordered[j].ProjectName]
		})
	}
	rows := make([][]string, 0, len(ordered))
	for _, entry := range ordered {
		framework := entry.Framework
		if framework == "" {
			framework = "unknown"
		}
		status := "unknown"
		if running != nil {
			status = "stopped"
			if running[entry.ProjectName] {
				status = "running"
			}
		}
		rows = append(rows, []string{entry.ProjectName, status, framework, entry.Domain, entry.Path})
	}
	return rows
}
