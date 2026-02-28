package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage local snapshots for database and media",
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a snapshot of local database and media",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()

		name := ""
		if len(args) == 1 {
			name = args[0]
		}

		path, err := engine.CreateSnapshot(cwd, config, name)
		if err != nil {
			return fmt.Errorf("snapshot create failed: %w", err)
		}

		pterm.Success.Printf("Snapshot created at %s\n", path)
		return nil
	},
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		snapshots, err := engine.ListSnapshots(cwd)
		if err != nil {
			return fmt.Errorf("snapshot list failed: %w", err)
		}

		if len(snapshots) == 0 {
			pterm.Info.Println("No snapshots found.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NAME\tCREATED_AT\tDB\tMEDIA")
		for _, snapshot := range snapshots {
			created := "-"
			if !snapshot.CreatedAt.IsZero() {
				created = snapshot.CreatedAt.Format("2006-01-02 15:04:05")
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%t\t%t\n", snapshot.Name, created, snapshot.DB, snapshot.Media)
		}
		_ = w.Flush()
		return nil
	},
}

var snapshotRestoreCmd = &cobra.Command{
	Use:   "restore <name>",
	Short: "Restore a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()
		name := args[0]

		dbOnly, _ := cmd.Flags().GetBool("db-only")
		mediaOnly, _ := cmd.Flags().GetBool("media-only")
		if dbOnly && mediaOnly {
			return fmt.Errorf("cannot use --db-only and --media-only together")
		}

		if err := engine.RestoreSnapshot(cwd, config, name, dbOnly, mediaOnly); err != nil {
			return fmt.Errorf("snapshot restore failed: %w", err)
		}

		pterm.Success.Printf("Snapshot %s restored.\n", name)
		return nil
	},
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		name := args[0]

		if err := engine.DeleteSnapshot(cwd, name); err != nil {
			return err
		}

		pterm.Success.Printf("Snapshot %s deleted.\n", name)
		return nil
	},
}

var snapshotExportCmd = &cobra.Command{
	Use:   "export <name> [file]",
	Short: "Export a snapshot to a tar.gz file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		name := args[0]
		target := ""
		if len(args) == 2 {
			target = args[1]
		}

		if err := engine.ExportSnapshot(cwd, name, target); err != nil {
			return err
		}

		pterm.Success.Println("Snapshot exported successfully.")
		return nil
	},
}

func init() {
	snapshotRestoreCmd.Flags().Bool("db-only", false, "Restore database only")
	snapshotRestoreCmd.Flags().Bool("media-only", false, "Restore media only")

	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	snapshotCmd.AddCommand(snapshotExportCmd)

	rootCmd.AddCommand(snapshotCmd)
}
