package cmd

import (
	"fmt"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var blueprintCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "blueprint",
	Short: "Manage blueprint components and registry",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var blueprintCacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage remote blueprint registry cache",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var blueprintCacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached remote blueprints",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := engine.ListBlueprintCache()
		if err != nil {
			return fmt.Errorf("list cache: %w", err)
		}

		if len(entries) == 0 {
			pterm.Info.Println("Blueprint registry cache is empty.")
			return nil
		}

		table := pterm.TableData{{"Cache Key", "Last Used/Fetched", "Path"}}
		for _, entry := range entries {
			table = append(table, []string{
				entry.Key,
				entry.CachedAt.Format("2006-01-02 15:04:05"),
				entry.Path,
			})
		}

		if err := pterm.DefaultTable.WithHasHeader().WithData(table).Render(); err != nil {
			return err
		}

		return nil
	},
}

var blueprintCacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the remote blueprint registry cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := engine.ClearBlueprintCache(); err != nil {
			return fmt.Errorf("clear cache: %w", err)
		}
		pterm.Success.Println("Blueprint registry cache cleared.")
		return nil
	},
}

func init() {
	blueprintCacheCmd.AddCommand(blueprintCacheListCmd)
	blueprintCacheCmd.AddCommand(blueprintCacheClearCmd)
	blueprintCmd.AddCommand(blueprintCacheCmd)
}
