package cmd

import (
	"fmt"
	"os"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync [flags]",
	Short: "Synchronize files, media, and databases between environments",
	Long: `Synchronize your local development environment with a remote server (e.g., staging, production).
This command uses rsync for file/media transfers and mysqldump/mysql for database synchronization.
It supports bi-directional sync (local to remote, remote to local), but prevents accidental
overwrites on protected remotes.

Framework Notes:
- Magento 2: Media sync defaults to 'pub/media', excluding large generated/cache directories.
- Laravel: File sync includes 'storage/app/public' if media is requested.
- General: You can use --include/--exclude to fine-tune rsync behavior.

Case Studies:
- Daily Data Refresh: Fetch the latest DB and media from staging to work on a fresh dataset.
- Single File Fix: Push a hotfix file to a dev remote for quick verification.
- Media Sync Only: Sync product images from production without affecting your local code or DB.
- Full Onboarding: Use --full to get code, media, and DB in one command.`,
	Example: `  # Sync everything from staging to local (default behavior)
  govard sync --source staging --full

  # Sync only the database from production
  govard sync --source prod --db

  # Sync media from dev, but exclude specific folders
  govard sync --source dev --media --exclude "catalog/product/cache/*"

  # Sync a specific path (e.g., a theme folder) from local to dev
  govard sync --destination dev --file --path "app/design/frontend/MyTheme"

  # Dry-run: Show what would be synced without actually doing it
  govard sync --source staging --full --plan`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""

		source, _ := cmd.Flags().GetString("source")
		destination, _ := cmd.Flags().GetString("destination")
		files, _ := cmd.Flags().GetBool("file")
		media, _ := cmd.Flags().GetBool("media")
		database, _ := cmd.Flags().GetBool("db")
		full, _ := cmd.Flags().GetBool("full")
		deleteFiles, _ := cmd.Flags().GetBool("delete")
		path, _ := cmd.Flags().GetString("path")
		planOnly, _ := cmd.Flags().GetBool("plan")
		resume, _ := cmd.Flags().GetBool("resume")
		noResume, _ := cmd.Flags().GetBool("no-resume")
		noCompress, _ := cmd.Flags().GetBool("no-compress")
		includePatternsRaw, _ := cmd.Flags().GetStringArray("include")
		excludePatternsRaw, _ := cmd.Flags().GetStringArray("exclude")
		includePatterns := normalizeSyncPatterns(includePatternsRaw)
		excludePatterns := normalizeSyncPatterns(excludePatternsRaw)
		resumeTransfers := resolveSyncResumeMode(resume, noResume)

		if source == "" {
			source = "staging"
		}
		if destination == "" {
			destination = "local"
		}
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
			writeOperationEventBestEffort(
				"sync.run",
				operationStatus,
				config,
				source,
				destination,
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if err == nil {
				cwd, _ := os.Getwd()
				trackProjectRegistryBestEffort(config, cwd, "sync")
			}
		}()
		auditStatus := remote.RemoteAuditStatusFailure
		auditCategory := ""
		auditMessage := ""
		defer func() {
			if err != nil && auditMessage == "" {
				auditMessage = err.Error()
			}
			if err == nil && auditStatus == remote.RemoteAuditStatusFailure {
				auditStatus = remote.RemoteAuditStatusSuccess
			}
			if err != nil && auditCategory == "" {
				auditCategory = remote.ClassifyFailure(err, err.Error()).Category
			}
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:   "sync.run",
				Status:      auditStatus,
				Category:    auditCategory,
				Source:      source,
				Destination: destination,
				DurationMS:  time.Since(startedAt).Milliseconds(),
				Message:     auditMessage,
			})
		}()

		if full {
			files = true
			media = true
			database = true
		}
		if !files && !media && !database {
			files = true
		}

		plan := remote.BuildSyncPlan(remote.SyncOptions{
			Source:      source,
			Destination: destination,
			Files:       files,
			Media:       media,
			DB:          database,
			Delete:      deleteFiles,
			Resume:      resumeTransfers,
			NoCompress:  noCompress,
			Path:        path,
			Include:     includePatterns,
			Exclude:     excludePatterns,
		})
		execOpts := syncExecutionOptions{
			Files:      files,
			Media:      media,
			DB:         database,
			Delete:     deleteFiles,
			Resume:     resumeTransfers,
			NoCompress: noCompress,
			Path:       path,
			Include:    includePatterns,
			Exclude:    excludePatterns,
		}

		endpoints, err := resolveSyncEndpoints(config, source, destination)
		if err != nil {
			if planOnly {
				for _, line := range buildFallbackSyncPlanSummary(source, destination, execOpts, plan, err) {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				auditStatus = remote.RemoteAuditStatusPlan
				auditMessage = "Synchronization plan created with fallback resolution."
				operationStatus = engine.OperationStatusPlan
				operationMessage = "Synchronization plan created with fallback resolution."
				return nil
			}
			return err
		}

		policyWarnings, err := evaluateSyncPolicy(endpoints, execOpts)
		if err != nil {
			return err
		}

		executionPlan, err := buildSyncExecutionPlan(config, endpoints, execOpts)
		if err != nil {
			return err
		}

		if planOnly {
			for _, line := range buildSyncPlanSummary(endpoints, executionPlan, execOpts, policyWarnings) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			auditStatus = remote.RemoteAuditStatusPlan
			auditMessage = "Synchronization plan created for review."
			operationStatus = engine.OperationStatusPlan
			operationMessage = "Synchronization plan created for review."
			return nil
		}

		for _, warning := range policyWarnings {
			pterm.Warning.Println(warning)
		}

		if err := engine.RunHooks(config, engine.HookPreSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-synchronization hooks failed to execute: %w", err)
		}

		for i, rsyncCmd := range executionPlan.RsyncCommands {
			spinner, _ := pterm.DefaultSpinner.Start(executionPlan.Descriptions[i])
			rsyncCmd.Stdout = cmd.OutOrStdout()
			rsyncCmd.Stderr = cmd.ErrOrStderr()
			if err := rsyncCmd.Run(); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to sync: %v", err))
				return fmt.Errorf("sync command failed: %w", err)
			}
			spinner.Success()
		}

		for _, dbAction := range executionPlan.DatabaseActions {
			if err := dbAction(); err != nil {
				return err
			}
		}

		if err := engine.RunHooks(config, engine.HookPostSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("post-synchronization hooks failed to execute: %w", err)
		}

		pterm.Success.Println("Synchronization successfully completed.")
		auditStatus = remote.RemoteAuditStatusSuccess
		auditMessage = "Synchronization successfully completed."
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "Synchronization successfully completed."
		return nil
	},
}

func init() {
	syncCmd.Flags().String("source", "staging", "Source environment")
	syncCmd.Flags().String("destination", "local", "Destination environment")
	syncCmd.Flags().Bool("file", false, "Sync source code/files")
	syncCmd.Flags().Bool("media", false, "Sync media files")
	syncCmd.Flags().Bool("db", false, "Sync database")
	syncCmd.Flags().Bool("full", false, "Sync files, media, and database")
	syncCmd.Flags().Bool("delete", false, "Delete destination files missing on source")
	syncCmd.Flags().Bool("resume", true, "Enable resumable rsync transfers (--partial --append-verify)")
	syncCmd.Flags().Bool("no-resume", false, "Disable resumable rsync transfers")
	syncCmd.Flags().Bool("no-compress", false, "Disable rsync compression")
	syncCmd.Flags().String("path", "", "Sync a specific path")
	syncCmd.Flags().StringArray("include", nil, "Rsync include pattern (repeatable)")
	syncCmd.Flags().StringArray("exclude", nil, "Rsync exclude pattern (repeatable)")
	syncCmd.Flags().Bool("plan", false, "Print the sync plan and exit")

	rootCmd.AddCommand(syncCmd)
}

// SyncCommand exposes the sync command for testing.
func SyncCommand() *cobra.Command {
	return syncCmd
}
