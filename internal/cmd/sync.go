package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		config := loadFullConfig()
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
				auditMessage = "sync plan generated with fallback endpoint resolution"
				operationStatus = engine.OperationStatusPlan
				operationMessage = "sync plan generated with fallback endpoint resolution"
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
			auditMessage = "sync plan generated"
			operationStatus = engine.OperationStatusPlan
			operationMessage = "sync plan generated"
			return nil
		}

		for _, warning := range policyWarnings {
			pterm.Warning.Println(warning)
		}

		if err := engine.RunHooks(config, engine.HookPreSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("pre-sync hooks failed: %w", err)
		}

		for i, rsyncCmd := range executionPlan.RsyncCommands {
			pterm.Info.Printf("Executing: %s\n", executionPlan.Descriptions[i])
			rsyncCmd.Stdout = cmd.OutOrStdout()
			rsyncCmd.Stderr = cmd.ErrOrStderr()
			if err := rsyncCmd.Run(); err != nil {
				return fmt.Errorf("sync command failed: %w", err)
			}
		}

		for _, dbAction := range executionPlan.DatabaseActions {
			if err := dbAction(); err != nil {
				return err
			}
		}

		if err := engine.RunHooks(config, engine.HookPostSync, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("post-sync hooks failed: %w", err)
		}

		pterm.Success.Println("Sync completed.")
		auditStatus = remote.RemoteAuditStatusSuccess
		auditMessage = "sync completed"
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "sync completed"
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

type syncEndpoint struct {
	Name      string
	IsLocal   bool
	RemoteCfg engine.RemoteConfig
	RootPath  string
	MediaPath string
}

type resolvedSyncEndpoints struct {
	Source      syncEndpoint
	Destination syncEndpoint
}

type syncExecutionOptions struct {
	Files      bool
	Media      bool
	DB         bool
	Delete     bool
	Resume     bool
	NoCompress bool
	Path       string
	Include    []string
	Exclude    []string
}

type syncExecutionPlan struct {
	Descriptions    []string
	RsyncCommands   []*exec.Cmd
	DatabaseActions []func() error
}

func resolveSyncEndpoints(config engine.Config, sourceName string, destinationName string) (resolvedSyncEndpoints, error) {
	cwd, _ := os.Getwd()

	source, err := resolveSyncEndpoint(config, sourceName, cwd)
	if err != nil {
		return resolvedSyncEndpoints{}, err
	}

	destination, err := resolveSyncEndpoint(config, destinationName, cwd)
	if err != nil {
		return resolvedSyncEndpoints{}, err
	}

	return resolvedSyncEndpoints{
		Source:      source,
		Destination: destination,
	}, nil
}

func resolveSyncEndpoint(config engine.Config, name string, cwd string) (syncEndpoint, error) {
	if name == "local" {
		return syncEndpoint{
			Name:      name,
			IsLocal:   true,
			RootPath:  cwd,
			MediaPath: engine.ResolveLocalMediaPath(config, cwd),
		}, nil
	}

	remoteCfg, err := ensureRemoteKnown(config, name)
	if err != nil {
		return syncEndpoint{}, err
	}

	root, media := engine.ResolveRemotePathsForConfig(config.Recipe, remoteCfg)
	if strings.TrimSpace(root) == "" {
		return syncEndpoint{}, fmt.Errorf("remote %s has empty project path", name)
	}

	return syncEndpoint{
		Name:      name,
		IsLocal:   false,
		RemoteCfg: remoteCfg,
		RootPath:  root,
		MediaPath: media,
	}, nil
}

func buildSyncExecutionPlan(config engine.Config, endpoints resolvedSyncEndpoints, opts syncExecutionOptions) (syncExecutionPlan, error) {
	plan := syncExecutionPlan{
		Descriptions: []string{},
	}

	if opts.Files {
		sourcePath := endpoints.Source.RootPath
		destinationPath := endpoints.Destination.RootPath
		if strings.TrimSpace(opts.Path) != "" {
			sourcePath = filepath.Join(endpoints.Source.RootPath, opts.Path)
			destinationPath = filepath.Join(endpoints.Destination.RootPath, opts.Path)
			if endpoints.Destination.IsLocal {
				if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
					return syncExecutionPlan{}, fmt.Errorf("failed to create destination parent directory: %w", err)
				}
			}
		}
		rsyncCmd, desc, err := buildRsyncForEndpoints(
			endpoints.Source,
			endpoints.Destination,
			sourcePath,
			destinationPath,
			opts.Delete,
			opts.Resume,
			opts.NoCompress,
			opts.Include,
			opts.Exclude,
		)
		if err != nil {
			return syncExecutionPlan{}, err
		}
		plan.RsyncCommands = append(plan.RsyncCommands, rsyncCmd)
		plan.Descriptions = append(plan.Descriptions, "[sync:files] "+desc)
	}

	if opts.Media {
		rsyncCmd, desc, err := buildRsyncForEndpoints(
			endpoints.Source,
			endpoints.Destination,
			endpoints.Source.MediaPath,
			endpoints.Destination.MediaPath,
			opts.Delete,
			opts.Resume,
			opts.NoCompress,
			opts.Include,
			opts.Exclude,
		)
		if err != nil {
			return syncExecutionPlan{}, err
		}
		plan.RsyncCommands = append(plan.RsyncCommands, rsyncCmd)
		plan.Descriptions = append(plan.Descriptions, "[sync:media] "+desc)
	}

	if opts.DB {
		dbDesc, action, err := buildDatabaseSyncAction(config, endpoints.Source, endpoints.Destination)
		if err != nil {
			return syncExecutionPlan{}, err
		}
		plan.Descriptions = append(plan.Descriptions, "[sync:db] "+dbDesc)
		plan.DatabaseActions = append(plan.DatabaseActions, action)
	}

	return plan, nil
}

func evaluateSyncPolicy(endpoints resolvedSyncEndpoints, opts syncExecutionOptions) ([]string, error) {
	if endpoints.Source.Name == endpoints.Destination.Name {
		return nil, fmt.Errorf("source and destination cannot be the same: %s", endpoints.Source.Name)
	}
	if endpoints.Source.IsLocal == endpoints.Destination.IsLocal {
		return nil, fmt.Errorf("sync currently supports local<->remote only")
	}
	if !endpoints.Destination.IsLocal {
		if blocked, reason := engine.RemoteWriteBlocked(endpoints.Destination.RemoteCfg); blocked {
			return nil, fmt.Errorf("destination remote '%s' is write-protected: %s", endpoints.Destination.Name, reason)
		}
	}

	if opts.Files {
		if err := ensureSyncCapability(endpoints, engine.RemoteCapabilityFiles); err != nil {
			return nil, err
		}
	}
	if opts.Media {
		if err := ensureSyncCapability(endpoints, engine.RemoteCapabilityMedia); err != nil {
			return nil, err
		}
	}
	if opts.DB {
		if err := ensureSyncCapability(endpoints, engine.RemoteCapabilityDB); err != nil {
			return nil, err
		}
	}

	warnings := []string{}
	if opts.Path != "" && (opts.Media || opts.DB) {
		warnings = append(warnings, "--path applies to file sync only. media/db sync still use full configured paths.")
	}
	if len(opts.Include) > 0 && !opts.Files && !opts.Media {
		warnings = append(warnings, "--include patterns apply to file/media rsync scopes only.")
	}
	if len(opts.Exclude) > 0 && !opts.Files && !opts.Media {
		warnings = append(warnings, "--exclude patterns apply to file/media rsync scopes only.")
	}
	if opts.Resume && !opts.Files && !opts.Media {
		warnings = append(warnings, "--resume applies to file/media rsync scopes only.")
	}
	if opts.NoCompress && !opts.Files && !opts.Media {
		warnings = append(warnings, "--no-compress applies to file/media rsync scopes only.")
	}
	if !endpoints.Destination.IsLocal {
		warnings = append(warnings, fmt.Sprintf("This operation writes to remote destination '%s'.", endpoints.Destination.Name))
	}
	if opts.Delete {
		warnings = append(warnings, "--delete is enabled and will remove destination files that do not exist in source.")
	}
	if opts.DB {
		warnings = append(warnings, "Database sync overwrites data on destination database.")
	}
	return warnings, nil
}

func buildSyncPlanSummary(endpoints resolvedSyncEndpoints, execution syncExecutionPlan, opts syncExecutionOptions, warnings []string) []string {
	lines := []string{
		"Sync Plan Summary",
		fmt.Sprintf("source: %s", describeSyncEndpoint(endpoints.Source)),
		fmt.Sprintf("destination: %s", describeSyncEndpoint(endpoints.Destination)),
		fmt.Sprintf("scopes: %s", strings.Join(syncScopes(opts), ", ")),
		fmt.Sprintf("path filter: %s", syncPathFilter(opts.Path)),
		fmt.Sprintf("include patterns: %s", syncPatternSummary(opts.Include)),
		fmt.Sprintf("exclude patterns: %s", syncPatternSummary(opts.Exclude)),
		fmt.Sprintf("resume mode: %s", boolLabel(opts.Resume, "enabled", "disabled")),
		fmt.Sprintf("compression: %s", boolLabel(!opts.NoCompress, "enabled", "disabled")),
		fmt.Sprintf("delete mode: %s", boolLabel(opts.Delete, "enabled", "disabled")),
	}

	risk, reasons := syncRiskLevel(endpoints, opts)
	lines = append(lines, fmt.Sprintf("risk: %s (%s)", risk, strings.Join(reasons, "; ")))

	if len(warnings) > 0 {
		lines = append(lines, "policy warnings:")
		for _, warning := range warnings {
			lines = append(lines, " - "+warning)
		}
	}

	lines = append(lines, "planned steps:")
	if len(execution.Descriptions) == 0 {
		lines = append(lines, " - no transfer actions selected")
		return lines
	}
	for i, description := range execution.Descriptions {
		lines = append(lines, fmt.Sprintf(" %d. %s", i+1, description))
	}
	return lines
}

func buildFallbackSyncPlanSummary(source, destination string, opts syncExecutionOptions, legacy remote.SyncPlan, resolveErr error) []string {
	lines := []string{
		"Sync Plan Summary",
		fmt.Sprintf("source: %s", source),
		fmt.Sprintf("destination: %s", destination),
		fmt.Sprintf("scopes: %s", strings.Join(syncScopes(opts), ", ")),
		fmt.Sprintf("path filter: %s", syncPathFilter(opts.Path)),
		fmt.Sprintf("include patterns: %s", syncPatternSummary(opts.Include)),
		fmt.Sprintf("exclude patterns: %s", syncPatternSummary(opts.Exclude)),
		fmt.Sprintf("resume mode: %s", boolLabel(opts.Resume, "enabled", "disabled")),
		fmt.Sprintf("compression: %s", boolLabel(!opts.NoCompress, "enabled", "disabled")),
		fmt.Sprintf("delete mode: %s", boolLabel(opts.Delete, "enabled", "disabled")),
		"risk: medium (endpoint details unavailable)",
		fmt.Sprintf("warning: endpoint resolution failed: %v", resolveErr),
		"planned steps:",
		fmt.Sprintf(" 1. %s", legacy.Command),
	}
	return lines
}

func describeSyncEndpoint(endpoint syncEndpoint) string {
	if endpoint.IsLocal {
		return fmt.Sprintf("%s (local path: %s)", endpoint.Name, endpoint.RootPath)
	}
	writePolicy := "write-allowed"
	if blocked, reason := engine.RemoteWriteBlocked(endpoint.RemoteCfg); blocked {
		writePolicy = "write-blocked (" + reason + ")"
	}
	return fmt.Sprintf(
		"%s (remote: %s, env: %s, capabilities: %s, policy: %s, root: %s)",
		endpoint.Name,
		remote.RemoteTarget(endpoint.RemoteCfg),
		endpoint.RemoteCfg.Environment,
		strings.Join(engine.RemoteCapabilityList(endpoint.RemoteCfg), ","),
		writePolicy,
		endpoint.RootPath,
	)
}

func syncScopes(opts syncExecutionOptions) []string {
	scopes := []string{}
	if opts.Files {
		scopes = append(scopes, "files")
	}
	if opts.Media {
		scopes = append(scopes, "media")
	}
	if opts.DB {
		scopes = append(scopes, "db")
	}
	if len(scopes) == 0 {
		return []string{"none"}
	}
	return scopes
}

func syncPathFilter(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "(none)"
	}
	return trimmed
}

func syncRiskLevel(endpoints resolvedSyncEndpoints, opts syncExecutionOptions) (string, []string) {
	reasons := []string{"standard file synchronization"}
	if !endpoints.Destination.IsLocal {
		reasons = append(reasons, "remote destination write")
	}
	if opts.DB {
		reasons = append(reasons, "database overwrite")
	}
	if opts.Delete {
		reasons = append(reasons, "delete mode")
	}

	switch {
	case opts.DB || opts.Delete:
		return "high", reasons
	case !endpoints.Destination.IsLocal:
		return "medium", reasons
	default:
		return "low", reasons
	}
}

func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func ensureSyncCapability(endpoints resolvedSyncEndpoints, capability string) error {
	if err := ensureEndpointCapability(endpoints.Source, "source", capability); err != nil {
		return err
	}
	if err := ensureEndpointCapability(endpoints.Destination, "destination", capability); err != nil {
		return err
	}
	return nil
}

func ensureEndpointCapability(endpoint syncEndpoint, position string, capability string) error {
	if endpoint.IsLocal {
		return nil
	}
	if engine.RemoteCapabilityEnabled(endpoint.RemoteCfg, capability) {
		return nil
	}
	return fmt.Errorf(
		"%s remote '%s' does not allow '%s' operations (capabilities: %s)",
		position,
		endpoint.Name,
		capability,
		strings.Join(engine.RemoteCapabilityList(endpoint.RemoteCfg), ","),
	)
}

func buildRsyncForEndpoints(
	source syncEndpoint,
	destination syncEndpoint,
	sourcePath string,
	destinationPath string,
	deleteFiles bool,
	resume bool,
	noCompress bool,
	includePatterns []string,
	excludePatterns []string,
) (*exec.Cmd, string, error) {
	if source.IsLocal == destination.IsLocal {
		return nil, "", fmt.Errorf("sync only supports local<->remote transfers")
	}

	if source.IsLocal {
		cmd := remote.BuildRsyncCommand(
			destination.Name,
			ensureTrailingSlash(sourcePath),
			remote.RemoteTarget(destination.RemoteCfg)+":"+ensureTrailingSlash(destinationPath),
			destination.RemoteCfg,
			deleteFiles,
			resume,
			noCompress,
			includePatterns,
			excludePatterns,
		)
		return cmd, cmd.String(), nil
	}

	cmd := remote.BuildRsyncCommand(
		source.Name,
		remote.RemoteTarget(source.RemoteCfg)+":"+ensureTrailingSlash(sourcePath),
		ensureTrailingSlash(destinationPath),
		source.RemoteCfg,
		deleteFiles,
		resume,
		noCompress,
		includePatterns,
		excludePatterns,
	)
	return cmd, cmd.String(), nil
}

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

func normalizeSyncPatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func syncPatternSummary(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ",")
}

func resolveSyncResumeMode(resume bool, noResume bool) bool {
	if noResume {
		return false
	}
	return resume
}

func buildDatabaseSyncAction(config engine.Config, source syncEndpoint, destination syncEndpoint) (string, func() error, error) {
	localDBContainer := fmt.Sprintf("%s-db-1", config.ProjectName)
	localCredentials := resolveLocalDBCredentials(localDBContainer)

	switch {
	case !source.IsLocal && destination.IsLocal:
		desc := fmt.Sprintf("ssh %s \"mysqldump ...\" | docker exec -i %s mysql ...", remote.RemoteTarget(source.RemoteCfg), localDBContainer)
		return desc, func() error {
			remoteCredentials, probeErr := resolveRemoteDBCredentials(config, source.Name, source.RemoteCfg)
			if probeErr != nil {
				pterm.Warning.Println(formatRemoteDBProbeWarning(source.Name, probeErr))
			}
			dumpCmd := remote.BuildSSHExecCommand(source.Name, source.RemoteCfg, true, buildRemoteMySQLDumpCommandString(remoteCredentials, false))
			importCmd := buildLocalDBImportCommand(localDBContainer, localCredentials)
			return RunDumpToImport(dumpCmd, importCmd, true, os.Stdout, os.Stderr)
		}, nil
	case source.IsLocal && !destination.IsLocal:
		desc := fmt.Sprintf("docker exec -i %s mysqldump ... | ssh %s \"mysql ...\"", localDBContainer, remote.RemoteTarget(destination.RemoteCfg))
		return desc, func() error {
			dumpCmd := buildLocalDBDumpCommand(localDBContainer, localCredentials, false)
			remoteCredentials, probeErr := resolveRemoteDBCredentials(config, destination.Name, destination.RemoteCfg)
			if probeErr != nil {
				pterm.Warning.Println(formatRemoteDBProbeWarning(destination.Name, probeErr))
			}
			importCmd := remote.BuildSSHExecCommand(destination.Name, destination.RemoteCfg, true, buildRemoteMySQLImportCommandString(remoteCredentials))
			return RunDumpToImport(dumpCmd, importCmd, true, os.Stdout, os.Stderr)
		}, nil
	default:
		return "", nil, fmt.Errorf("database sync only supports local<->remote transfers")
	}
}

func pipeCommands(producer *exec.Cmd, consumer *exec.Cmd) error {
	pipeReader, pipeWriter := io.Pipe()
	producer.Stdout = pipeWriter
	producer.Stderr = os.Stderr
	consumer.Stdin = pipeReader
	consumer.Stdout = os.Stdout
	consumer.Stderr = os.Stderr

	if err := producer.Start(); err != nil {
		_ = pipeWriter.Close()
		_ = pipeReader.Close()
		return err
	}
	if err := consumer.Start(); err != nil {
		_ = pipeWriter.Close()
		_ = pipeReader.Close()
		_ = producer.Wait()
		return err
	}

	producerErr := producer.Wait()
	_ = pipeWriter.Close()
	consumerErr := consumer.Wait()
	_ = pipeReader.Close()

	if producerErr != nil {
		return producerErr
	}
	return consumerErr
}
