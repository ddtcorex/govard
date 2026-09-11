package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var snapshotCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:     "snapshot",
	Aliases: []string{"snap"},
	Short:   "Manage local snapshots for database and media",
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

		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))

		if environment != "" && environment != "local" {
			local, _ := cmd.Flags().GetBool("local")
			return runRemoteSnapshotCreate(cmd, config, environment, name, local)
		}

		path, err := engine.CreateSnapshot(cwd, config, name)
		if err != nil {
			return fmt.Errorf("snapshot create failed: %w", err)
		}

		pterm.Success.Printf("Snapshot created at %s\n", path)
		return nil
	},
}

func runRemoteSnapshotCreate(cmd *cobra.Command, config engine.Config, envName string, name string, local bool) (err error) {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationCategory := ""
	operationMessage := ""

	defer func() {
		if err != nil && operationMessage == "" {
			operationMessage = err.Error()
		}
		if err == nil && operationStatus == engine.OperationStatusFailure {
			operationStatus = engine.OperationStatusSuccess
		}
		if err != nil && operationCategory == "" {
			operationCategory = classifyCommandError(err)
		}
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "snapshot.create",
			Status:     auditStatusFromEngine(operationStatus),
			Category:   operationCategory,
			Remote:     envName,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    operationMessage,
		})
	}()

	if name == "" {
		name = time.Now().Format("20060102-150405")
	}

	if err := remote.ValidateSnapshotName(name); err != nil {
		return err
	}

	remoteName, remoteCfg, err := ensureRemoteKnown(config, envName)
	if err != nil {
		return err
	}

	// Must have DB cap
	if !engine.RemoteCapabilityEnabled(remoteCfg, engine.RemoteCapabilityDB) {
		return fmt.Errorf("remote '%s' does not allow db operations", remoteName)
	}

	// Probe DB credentials
	var credentials dbCredentials
	var probeErr error
	if config.Framework != "none" {
		credentials, probeErr = resolveRemoteDBCredentials(config, remoteName, remoteCfg)
		if probeErr != nil {
			pterm.Warning.Println(formatRemoteDBProbeWarning(remoteName, probeErr))
		}
	}

	dbDumpCommandStr := ""
	if config.Framework != "none" {
		dbDumpCommandStr = buildRemoteMySQLDumpCommandString(credentials, false, false, config.Framework, true)
	}

	_, mediaPath := engine.ResolveRemotePathsForConfig(config.Framework, remoteCfg)

	createCmdStr := remote.BuildRemoteSnapshotCreateCommand(remoteCfg, name, config.Framework, dbDumpCommandStr, mediaPath)

	if local {
		return fmt.Errorf("--local mode is not yet implemented for remote snapshots")
	}

	pterm.Info.Printf("Creating snapshot %s on remote %s...\n", name, remoteName)
	sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, createCmdStr)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("remote snapshot creation failed: %w", err)
	}

	pterm.Success.Printf("Remote snapshot created at %s\n", remote.RemoteSnapshotDir(remoteCfg, name))
	operationMessage = "remote snapshot created"
	return nil
}

func auditStatusFromEngine(status engine.OperationStatus) string {
	if status == engine.OperationStatusSuccess {
		return remote.RemoteAuditStatusSuccess
	}
	return remote.RemoteAuditStatusFailure
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))
		if environment != "" && environment != "local" {
			config, err := loadFullConfig()
			if err != nil {
				return err
			}
			return runRemoteSnapshotList(cmd, config, environment)
		}

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
		_, _ = fmt.Fprintln(w, "NAME\tCREATED_AT\tSIZE\tDB\tMEDIA")
		for _, snapshot := range snapshots {
			created := "-"
			if !snapshot.CreatedAt.IsZero() {
				created = snapshot.CreatedAt.Format("2006-01-02 15:04:05")
			}
			sizeStr := formatBytes(snapshot.SizeBytes)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\n", snapshot.Name, created, sizeStr, snapshot.DB, snapshot.Media)
		}
		_ = w.Flush()
		return nil
	},
}

func runRemoteSnapshotList(cmd *cobra.Command, config engine.Config, envName string) error {
	remoteName, remoteCfg, err := ensureRemoteKnown(config, envName)
	if err != nil {
		return err
	}

	listCmdStr := remote.BuildRemoteSnapshotListCommand(remoteCfg)
	sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, false, listCmdStr)

	output, err := sshCmd.Output()
	if err != nil {
		return fmt.Errorf("remote snapshot list failed: %w", err)
	}

	snapshots, err := remote.ParseRemoteSnapshotList(string(output))
	if err != nil {
		return fmt.Errorf("failed to parse remote snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		pterm.Info.Printf("No snapshots found on remote %s.\n", remoteName)
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
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
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

		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))
		if environment != "" && environment != "local" {
			return runRemoteSnapshotRestore(cmd, config, environment, name, dbOnly, mediaOnly)
		}

		if err := engine.RestoreSnapshot(cwd, config, name, dbOnly, mediaOnly); err != nil {
			return fmt.Errorf("snapshot restore failed: %w", err)
		}

		pterm.Success.Printf("Snapshot %s restored.\n", name)
		return nil
	},
}

func runRemoteSnapshotRestore(cmd *cobra.Command, config engine.Config, envName string, name string, dbOnly, mediaOnly bool) (err error) {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationCategory := ""
	operationMessage := ""

	defer func() {
		if err != nil && operationMessage == "" {
			operationMessage = err.Error()
		}
		if err == nil && operationStatus == engine.OperationStatusFailure {
			operationStatus = engine.OperationStatusSuccess
		}
		if err != nil && operationCategory == "" {
			operationCategory = classifyCommandError(err)
		}
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "snapshot.restore",
			Status:     auditStatusFromEngine(operationStatus),
			Category:   operationCategory,
			Remote:     envName,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    operationMessage,
		})
	}()

	if err := remote.ValidateSnapshotName(name); err != nil {
		return err
	}

	remoteName, remoteCfg, err := ensureRemoteKnown(config, envName)
	if err != nil {
		return err
	}

	if blocked, reason := engine.RemoteWriteBlocked(remoteName, remoteCfg); blocked {
		return fmt.Errorf("remote environment '%s' is write-protected: %s", remoteName, reason)
	}

	var credentials dbCredentials
	var probeErr error
	if config.Framework != "none" && !mediaOnly {
		credentials, probeErr = resolveRemoteDBCredentials(config, remoteName, remoteCfg)
		if probeErr != nil {
			pterm.Warning.Println(formatRemoteDBProbeWarning(remoteName, probeErr))
		}
	}

	dbImportCommandStr := ""
	if config.Framework != "none" && !mediaOnly {
		dbImportCommandStr = buildRemoteMySQLImportCommandString(credentials)
	}

	_, mediaPath := engine.ResolveRemotePathsForConfig(config.Framework, remoteCfg)

	restoreCmdStr := remote.BuildRemoteSnapshotRestoreCommand(remoteCfg, name, config.Framework, dbImportCommandStr, mediaPath, dbOnly, mediaOnly)

	pterm.Info.Printf("Restoring snapshot %s on remote %s...\n", name, remoteName)
	sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, restoreCmdStr)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("remote snapshot restore failed: %w", err)
	}

	pterm.Success.Printf("Remote snapshot %s restored.\n", name)
	operationMessage = "remote snapshot restored"
	return nil
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))

		cwd, _ := os.Getwd()
		name := args[0]

		if environment != "" && environment != "local" {
			config, err := loadFullConfig()
			if err != nil {
				return err
			}
			return runRemoteSnapshotDelete(cmd, config, environment, name)
		}

		if err := engine.DeleteSnapshot(cwd, name); err != nil {
			return err
		}

		pterm.Success.Printf("Snapshot %s deleted.\n", name)
		return nil
	},
}

func runRemoteSnapshotDelete(cmd *cobra.Command, config engine.Config, envName string, name string) (err error) {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationCategory := ""
	operationMessage := ""

	defer func() {
		if err != nil && operationMessage == "" {
			operationMessage = err.Error()
		}
		if err == nil && operationStatus == engine.OperationStatusFailure {
			operationStatus = engine.OperationStatusSuccess
		}
		if err != nil && operationCategory == "" {
			operationCategory = classifyCommandError(err)
		}
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "snapshot.delete",
			Status:     auditStatusFromEngine(operationStatus),
			Category:   operationCategory,
			Remote:     envName,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    operationMessage,
		})
	}()

	if err := remote.ValidateSnapshotName(name); err != nil {
		return err
	}

	remoteName, remoteCfg, err := ensureRemoteKnown(config, envName)
	if err != nil {
		return err
	}

	deleteCmdStr := remote.BuildRemoteSnapshotDeleteCommand(remoteCfg, name)
	sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, deleteCmdStr)

	pterm.Info.Printf("Deleting snapshot %s on remote %s...\n", name, remoteName)
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remote snapshot delete failed: %w: %s", err, string(output))
	}

	pterm.Success.Printf("Remote snapshot %s deleted.\n", name)
	operationMessage = "remote snapshot deleted"
	return nil
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

var snapshotPullCmd = &cobra.Command{
	Use:   "pull <name>",
	Short: "Pull a snapshot from a remote environment to local",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))
		if environment == "" || environment == "local" {
			return fmt.Errorf("pull requires a remote --environment")
		}

		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		name := args[0]
		if err := remote.ValidateSnapshotName(name); err != nil {
			return err
		}

		remoteName, remoteCfg, err := ensureRemoteKnown(config, environment)
		if err != nil {
			return err
		}

		cwd, _ := os.Getwd()
		localSnapshotDir := filepath.Join(engine.SnapshotRoot(cwd), name)

		pterm.Info.Printf("Pulling snapshot %s from %s...\n", name, remoteName)

		startedAt := time.Now()
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""
		defer func() {
			if err != nil && operationMessage == "" {
				operationMessage = err.Error()
			}
			if err == nil && operationStatus == engine.OperationStatusFailure {
				operationStatus = engine.OperationStatusSuccess
			}
			if err != nil && operationCategory == "" {
				operationCategory = classifyCommandError(err)
			}
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:   "snapshot.pull",
				Status:      auditStatusFromEngine(operationStatus),
				Category:    operationCategory,
				Remote:      remoteName,
				Source:      remoteName,
				Destination: "local",
				DurationMS:  time.Since(startedAt).Milliseconds(),
				Message:     operationMessage,
			})
		}()

		rsyncCmd := remote.BuildRemoteSnapshotPullCommand(remoteName, remoteCfg, name, localSnapshotDir)
		rsyncCmd.Stdout = os.Stdout
		rsyncCmd.Stderr = os.Stderr

		if err := rsyncCmd.Run(); err != nil {
			err = fmt.Errorf("remote snapshot pull failed: %w", err)
			return err
		}

		pterm.Success.Printf("Snapshot %s pulled successfully.\n", name)
		operationMessage = "remote snapshot pulled"
		return nil
	},
}

var snapshotPushCmd = &cobra.Command{
	Use:   "push <name>",
	Short: "Push a local snapshot to a remote environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, _ := cmd.Flags().GetString("environment")
		environment = strings.ToLower(strings.TrimSpace(environment))
		if environment == "" || environment == "local" {
			return fmt.Errorf("push requires a remote --environment")
		}

		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		name := args[0]
		if err := remote.ValidateSnapshotName(name); err != nil {
			return err
		}

		remoteName, remoteCfg, err := ensureRemoteKnown(config, environment)
		if err != nil {
			return err
		}

		if blocked, reason := engine.RemoteWriteBlocked(remoteName, remoteCfg); blocked {
			return fmt.Errorf("remote environment '%s' is write-protected: %s", remoteName, reason)
		}

		cwd, _ := os.Getwd()
		localSnapshotDir := filepath.Join(engine.SnapshotRoot(cwd), name)

		if _, err := os.Stat(localSnapshotDir); os.IsNotExist(err) {
			return fmt.Errorf("local snapshot '%s' does not exist", name)
		}

		// Ensure the parent directory exists on the remote
		parentDir := remote.QuoteRemotePath(remote.RemoteSnapshotRoot(remoteCfg))
		mkdirCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, "mkdir -p "+parentDir)
		_ = mkdirCmd.Run()

		pterm.Info.Printf("Pushing snapshot %s to %s...\n", name, remoteName)

		startedAt := time.Now()
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""
		defer func() {
			if err != nil && operationMessage == "" {
				operationMessage = err.Error()
			}
			if err == nil && operationStatus == engine.OperationStatusFailure {
				operationStatus = engine.OperationStatusSuccess
			}
			if err != nil && operationCategory == "" {
				operationCategory = classifyCommandError(err)
			}
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:   "snapshot.push",
				Status:      auditStatusFromEngine(operationStatus),
				Category:    operationCategory,
				Remote:      remoteName,
				Source:      "local",
				Destination: remoteName,
				DurationMS:  time.Since(startedAt).Milliseconds(),
				Message:     operationMessage,
			})
		}()

		rsyncCmd := remote.BuildRemoteSnapshotPushCommand(remoteName, remoteCfg, name, localSnapshotDir)
		rsyncCmd.Stdout = os.Stdout
		rsyncCmd.Stderr = os.Stderr

		if err := rsyncCmd.Run(); err != nil {
			err = fmt.Errorf("remote snapshot push failed: %w", err)
			return err
		}

		pterm.Success.Printf("Snapshot %s pushed successfully.\n", name)
		operationMessage = "remote snapshot pushed"
		return nil
	},
}

func init() {
	snapshotCmd.PersistentFlags().StringP("environment", "e", "", "Target environment (local, staging, prod, etc.)")

	snapshotCreateCmd.Flags().Bool("local", false, "Stream the remote snapshot directly to the local machine")

	snapshotRestoreCmd.Flags().Bool("db-only", false, "Restore database only")
	snapshotRestoreCmd.Flags().Bool("media-only", false, "Restore media only")

	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	snapshotCmd.AddCommand(snapshotExportCmd)
	snapshotCmd.AddCommand(snapshotPullCmd)
	snapshotCmd.AddCommand(snapshotPushCmd)

	rootCmd.AddCommand(snapshotCmd)
}
