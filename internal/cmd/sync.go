package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var syncCmd = &cobra.Command{
	Use:   "sync [flags]",
	Short: "Synchronize files, media, and databases between environments",
	Long: `Synchronize your local development environment with a remote server (e.g., staging, production).
This command uses rsync for file/media transfers and mysqldump/mysql for database synchronization.
It supports bi-directional sync (local to remote, remote to local), but prevents accidental
overwrites on protected remotes.
It will generate a detailed synchronization plan and prompt for confirmation before starting.
While syncing, it provides a live 10-line rolling progress of transferred files.

Framework Notes:
- Magento 2: Media sync defaults to 'pub/media', excluding large generated/cache directories.
- Laravel: File sync includes 'storage/app/public' if media is requested.
- General: You can use --include/--exclude to fine-tune rsync behavior.
- Single File: Use --path "path/to/file" to sync a specific file or directory.
- Preferred naming: use --source/--from and --destination/--to.
- -e / --environment remains a supported source-environment selector.

Media Sync Modes (--media):
- optimized: Default for remote. Skips heavy assets like products (Magento) or caches (WordPress).
- minimal: Syncs only non-binary assets (CSS, JS, Fonts). Ideal for frontend tasks.
- all: Truly all files (excluding common junk like .tmp or .bak).
- none: Explicitly skip media.

Case Studies:
- Daily Data Refresh: Fetch the latest DB and media from staging to work on a fresh dataset.
- Single File Fix: Push a hotfix file to a dev remote for quick verification.
- Media Sync Only: Sync media from production without affecting your local code or DB.
- Full Onboarding: Use --full to get code, media, and DB in one command.
- Frontend Refresh: Sync only CSS/JS static assets using --media minimal.`,
	Example: `  # Sync everything from staging to local (default behavior)
  govard sync -s staging --full

  # Sync only the database from dev
  govard sync -s dev --db

  # Sync DB, excluding noise tables (logs, caches, cron)
  govard sync -s dev --db --no-noise

  # Sync DB, excluding PII data
  govard sync -s dev --db --no-pii

  # Sync media from dev, but exclude specific folders
  govard sync -s dev --media --exclude "catalog/product/cache/*"

  # Push a specific path to a dev remote
  govard sync -d dev --file --path "app/design/frontend/MyTheme"

  # Sync a single file from production
  govard sync -s prod --file --path "app/etc/config.php"

  # Dry-run: Show what would be synced without actually doing it
  govard sync -s staging --full --plan

  # Non-interactive: Proceed without confirmation (useful for scripts)
  govard sync -s staging --full --yes`,
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
		from, _ := cmd.Flags().GetString("from")
		environment, _ := cmd.Flags().GetString("environment")
		if cmd.Flags().Changed("from") && !cmd.Flags().Changed("source") {
			source = from
		}
		if cmd.Flags().Changed("environment") && !cmd.Flags().Changed("source") && !cmd.Flags().Changed("from") {
			source = environment
		}

		destination, _ := cmd.Flags().GetString("destination")
		to, _ := cmd.Flags().GetString("to")
		if cmd.Flags().Changed("to") && !cmd.Flags().Changed("destination") {
			destination = to
		}
		files, _ := cmd.Flags().GetBool("file")
		database, _ := cmd.Flags().GetBool("db")
		full, _ := cmd.Flags().GetBool("full")
		deleteFiles, _ := cmd.Flags().GetBool("delete")
		path, _ := cmd.Flags().GetString("path")
		planOnly, _ := cmd.Flags().GetBool("plan")
		yes, _ := cmd.Flags().GetBool("yes")
		resume, _ := cmd.Flags().GetBool("resume")
		noResume, _ := cmd.Flags().GetBool("no-resume")
		noCompress, _ := cmd.Flags().GetBool("no-compress")
		noNoise, _ := cmd.Flags().GetBool("no-noise")
		noPII, _ := cmd.Flags().GetBool("no-pii")
		mediaMode, _ := cmd.Flags().GetString("media")
		mediaMode = resolveMediaModeFlagValue(cmd, mediaMode, args)
		includePatternsRaw, _ := cmd.Flags().GetStringArray("include")
		excludePatternsRaw, _ := cmd.Flags().GetStringArray("exclude")
		includePatterns := normalizeSyncPatterns(includePatternsRaw)
		excludePatterns := normalizeSyncPatterns(excludePatternsRaw)
		resumeTransfers := resolveSyncResumeMode(resume, noResume)

		if source != "local" {
			resolvedSource, err := ResolveAutoRemote(config, source)
			if err != nil {
				return err
			}
			source = resolvedSource
		}
		if destination == "" {
			destination = "local"
		}

		if destination != "local" {
			if remoteName, ok := findRemoteByNameOrEnvironment(config, destination); ok {
				destination = remoteName
			}
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
			mediaMode = MediaSyncAll
			database = true
		}
		if !files && mediaMode == "" && !database {
			files = true
		}

		plan := remote.BuildSyncPlan(remote.SyncOptions{
			Source:      source,
			Destination: destination,
			Files:       files,
			Media:       mediaMode,
			DB:          database,
			Delete:      deleteFiles,
			Resume:      resumeTransfers,
			NoCompress:  noCompress,
			NoNoise:     noNoise,
			NoPII:       noPII,
			Path:        path,
			Include:     includePatterns,
			Exclude:     excludePatterns,
		})
		execOpts := SyncExecutionOptions{
			Files:      files,
			Media:      mediaMode,
			DB:         database,
			Delete:     deleteFiles,
			Resume:     resumeTransfers,
			NoCompress: noCompress,
			NoNoise:    noNoise,
			NoPII:      noPII,
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

		if !yes {
			if !stdinIsTerminal() {
				return fmt.Errorf("confirmation required to proceed with synchronization plan; use -y to assume yes in non-interactive environments")
			}
			for _, line := range buildSyncPlanSummary(endpoints, executionPlan, execOpts, policyWarnings) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			fmt.Println()
			confirmed, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).WithDefaultText("Do you want to proceed with this synchronization?").Show()
			if !confirmed {
				return fmt.Errorf("synchronization cancelled by user")
			}
		}

		if yes {
			for _, warning := range policyWarnings {
				pterm.Warning.Println(warning)
			}
		}

		// Pre-flight check: offer SSH key copy if auth fails on remote endpoints
		if source != "local" {
			_ = offerSSHKeyCopyOnAuthFailure(source, config.Remotes[source])
		}
		if destination != "local" {
			_ = offerSSHKeyCopyOnAuthFailure(destination, config.Remotes[destination])
		}

		syncMessage := fmt.Sprintf("Synchronizing %s from '%s' to '%s'...", strings.Join(syncScopes(execOpts), ", "), source, destination)
		pterm.Info.Println(syncMessage)

		if err := engine.RunHooks(config, engine.HookPreSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-synchronization hooks failed to execute: %w", err)
		}

		for i, rsyncCmd := range executionPlan.RsyncCommands {
			description := executionPlan.Descriptions[i]
			pterm.Info.Println(description)

			area, _ := pterm.DefaultArea.Start()
			writer := newTailWriter(area, 10)
			rsyncCmd.Stdout = writer
			rsyncCmd.Stderr = writer

			if err := rsyncCmd.Run(); err != nil {
				_ = area.Stop()
				fmt.Println()
				cont, err := handleRsyncError(err, executionPlan.RsyncScopes[i])
				if !cont {
					return err
				}
				continue
			}

			_ = area.Stop()
			fmt.Println()
			pterm.Success.Printf("%s completed.\n", description)
		}

		for _, dbAction := range executionPlan.DatabaseActions {
			if err := dbAction(); err != nil {
				return err
			}
		}

		if err := engine.RunHooks(config, engine.HookPostSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("post-synchronization hooks failed to execute: %w", err)
		}

		if engine.IsMagento2Family(config.Framework) && (files || mediaMode != "") {
			_ = engine.FixProjectPermissions(config.ProjectName, config)
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
	syncCmd.Flags().SortFlags = false

	// 1. Source & Destination
	syncCmd.Flags().StringP("source", "s", "", "Source environment (default: auto-select staging or dev)")
	syncCmd.Flags().String("from", "", "Source environment alias for --source")
	syncCmd.Flags().StringP("environment", "e", "", "Source environment alias for --source")
	syncCmd.Flags().StringP("destination", "d", "local", "Destination environment")
	syncCmd.Flags().String("to", "", "Destination environment alias for --destination")

	// 2. Resource Scopes
	syncCmd.Flags().BoolP("full", "A", false, "Sync files, media, and database")
	syncCmd.Flags().BoolP("file", "f", false, "Sync source code/files")
	syncCmd.Flags().StringP("media", "m", "", "Sync media files (default: optimized when used without a value; modes: optimized, all, catalog, minimal, or none)")
	if mediaFlag := syncCmd.Flags().Lookup("media"); mediaFlag != nil {
		mediaFlag.NoOptDefVal = MediaSyncOptimized
	}
	syncCmd.Flags().BoolP("db", "b", false, "Sync database")

	// 3. Filters & Paths
	syncCmd.Flags().StringP("path", "p", "", "Sync a specific path")
	syncCmd.Flags().StringArrayP("include", "I", nil, "Rsync include pattern (repeatable)")
	syncCmd.Flags().StringArrayP("exclude", "X", nil, "Rsync exclude pattern (repeatable)")

	// 4. Database Privacy & Protection
	syncCmd.Flags().BoolP("no-noise", "N", false, "Exclude ephemeral/noise tables from database sync (logs, caches, etc)")
	syncCmd.Flags().BoolP("no-pii", "P", false, "Exclude PII/sensitive tables from database sync (users, orders, passwords, etc)")

	// 5. Transfer & Execution Control
	syncCmd.Flags().BoolP("delete", "D", false, "Delete destination files missing on source")
	syncCmd.Flags().BoolP("resume", "R", true, "Enable resumable rsync transfers (--partial --append-verify)")
	syncCmd.Flags().Bool("no-resume", false, "Disable resumable rsync transfers")
	syncCmd.Flags().BoolP("no-compress", "C", false, "Disable rsync compression")
	syncCmd.Flags().Bool("plan", false, "Print the sync plan and exit")
	syncCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and proceed with synchronization")

	rootCmd.AddCommand(syncCmd)
}

func handleRsyncError(err error, scope string) (bool, error) {
	if err == nil {
		return true, nil
	}

	exitCode := -1
	if exitError, ok := err.(*exec.ExitError); ok {
		exitCode = exitError.ExitCode()
	}

	if (exitCode == 23 || exitCode == 24) && scope == SyncScopeMedia {
		pterm.Warning.Printf("Media sync completed with partial errors (exit code %d). Some files may have been skipped due to system limits or vanished sources.\n", exitCode)
		return true, nil
	}

	if (exitCode == 23 || exitCode == 24) && scope == SyncScopeFiles {
		return false, fmt.Errorf("file synchronization failed with partial errors (exit code %d): %w", exitCode, err)
	}
	return false, fmt.Errorf("failed to sync: %w", err)
}

// HandleRsyncErrorForTest is a wrapper for testing internal error handling logic.
func HandleRsyncErrorForTest(err error, scope string) (bool, error) {
	return handleRsyncError(err, scope)
}

// SyncCommand exposes the sync command for testing.
func SyncCommand() *cobra.Command {
	return syncCmd
}

// ResetSyncFlagsForTest resets sync command flags to defaults for tests.
func ResetSyncFlagsForTest() {
	syncCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if sliceValue, ok := flag.Value.(pflag.SliceValue); ok && (flag.DefValue == "" || flag.DefValue == "[]") {
			_ = sliceValue.Replace(nil)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	})
}
