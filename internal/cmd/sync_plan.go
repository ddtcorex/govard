package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

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
		rsyncCmd, _, err := buildRsyncForEndpoints(
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
		plan.Descriptions = append(plan.Descriptions, "Syncing files and source code...")
	}

	if opts.Media {
		rsyncCmd, _, err := buildRsyncForEndpoints(
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
		plan.Descriptions = append(plan.Descriptions, "Syncing media and static assets...")
	}

	if opts.DB {
		_, action, err := buildDatabaseSyncAction(config, endpoints.Source, endpoints.Destination)
		if err != nil {
			return syncExecutionPlan{}, err
		}
		plan.Descriptions = append(plan.Descriptions, "Synchronizing database...")
		plan.DatabaseActions = append(plan.DatabaseActions, action)
	}

	return plan, nil
}

func evaluateSyncPolicy(endpoints resolvedSyncEndpoints, opts syncExecutionOptions) ([]string, error) {
	if endpoints.Source.Name == endpoints.Destination.Name {
		return nil, fmt.Errorf("source and destination must be different environments (both were '%s')", endpoints.Source.Name)
	}
	if endpoints.Source.IsLocal == endpoints.Destination.IsLocal {
		return nil, fmt.Errorf("synchronization is currently only supported between local and remote environments")
	}
	if !endpoints.Destination.IsLocal {
		if blocked, reason := engine.RemoteWriteBlocked(endpoints.Destination.Name, endpoints.Destination.RemoteCfg); blocked {
			return nil, fmt.Errorf("destination remote '%s' is Write-protected: %s", endpoints.Destination.Name, reason)
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
		warnings = append(warnings, "Path filter only applies to file synchronization; media and database will use full configured paths.")
	}
	if len(opts.Include) > 0 && !opts.Files && !opts.Media {
		warnings = append(warnings, "Include patterns are only applicable to file or media rsync operations.")
	}
	if len(opts.Exclude) > 0 && !opts.Files && !opts.Media {
		warnings = append(warnings, "Exclude patterns are only applicable to file or media rsync operations.")
	}
	if opts.Resume && !opts.Files && !opts.Media {
		warnings = append(warnings, "Resume mode is only applicable to file or media rsync operations.")
	}
	if opts.NoCompress && !opts.Files && !opts.Media {
		warnings = append(warnings, "Compression settings are only applicable to file or media rsync operations.")
	}
	if !endpoints.Destination.IsLocal {
		warnings = append(warnings, fmt.Sprintf("action Required: This operation will overwrite files on the remote destination '%s'", endpoints.Destination.Name))
	}
	if opts.Delete {
		warnings = append(warnings, "Caution: Delete mode is enabled. Files on the destination that do not exist on the source will be permanently removed.")
	}
	if opts.DB {
		warnings = append(warnings, "Warning: The destination database will be entirely overwritten with data from the source.")
	}
	return warnings, nil
}

func buildSyncPlanSummary(endpoints resolvedSyncEndpoints, execution syncExecutionPlan, opts syncExecutionOptions, warnings []string) []string {
	lines := []string{
		pterm.Bold.Sprint("Synchronization Plan Review"),
		fmt.Sprintf("  Source:      %s", describeSyncEndpoint(endpoints.Source)),
		fmt.Sprintf("  Destination: %s", describeSyncEndpoint(endpoints.Destination)),
		fmt.Sprintf("  Scopes:      %s", strings.Join(syncScopes(opts), ", ")),
		fmt.Sprintf("  Path Filter: %s", syncPathFilter(opts.Path)),
		fmt.Sprintf("  Includes:    %s", syncPatternSummary(opts.Include)),
		fmt.Sprintf("  Excludes:    %s", syncPatternSummary(opts.Exclude)),
		fmt.Sprintf("  Resume Mode: %s", boolLabel(opts.Resume, "Enabled", "Disabled")),
		fmt.Sprintf("  Compression: %s", boolLabel(!opts.NoCompress, "Enabled", "Disabled")),
		fmt.Sprintf("  Delete Mode: %s", boolLabel(opts.Delete, "Enabled (destructive)", "Disabled")),
	}

	risk, reasons := syncRiskLevel(endpoints, opts)
	lines = append(lines, fmt.Sprintf("  Risk Level:  %s (%s)", risk, strings.Join(reasons, "; ")))

	if len(warnings) > 0 {
		lines = append(lines, pterm.Yellow("Policy Warnings:"))
		for _, warning := range warnings {
			lines = append(lines, "  ! "+warning)
		}
	}

	lines = append(lines, "Planned Actions:")
	if len(execution.Descriptions) == 0 {
		lines = append(lines, "  (No transfer actions selected)")
		return lines
	}
	for i, description := range execution.Descriptions {
		lines = append(lines, fmt.Sprintf(" %d. %s", i+1, description))
		if i < len(execution.RsyncCommands) {
			lines = append(lines, fmt.Sprintf("    Command: %s", execution.RsyncCommands[i].String()))
		}
	}
	return lines
}

func buildFallbackSyncPlanSummary(source, destination string, opts syncExecutionOptions, legacy remote.SyncPlan, resolveErr error) []string {
	lines := []string{
		pterm.Bold.Sprint("Synchronization Plan Review (Fallback)"),
		fmt.Sprintf("  Source:      %s", source),
		fmt.Sprintf("  Destination: %s", destination),
		fmt.Sprintf("  Scopes:      %s", strings.Join(syncScopes(opts), ", ")),
		fmt.Sprintf("  Path Filter: %s", syncPathFilter(opts.Path)),
		fmt.Sprintf("  Includes:    %s", syncPatternSummary(opts.Include)),
		fmt.Sprintf("  Excludes:    %s", syncPatternSummary(opts.Exclude)),
		fmt.Sprintf("  Resume Mode: %s", boolLabel(opts.Resume, "Enabled", "Disabled")),
		fmt.Sprintf("  Compression: %s", boolLabel(!opts.NoCompress, "Enabled", "Disabled")),
		fmt.Sprintf("  Delete Mode: %s", boolLabel(opts.Delete, "Enabled (destructive)", "Disabled")),
		fmt.Sprintf("  Risk Level:  %s (Endpoint details unavailable)", pterm.Yellow("MEDIUM RISK")),
		pterm.Yellow(fmt.Sprintf("Warning: Full endpoint resolution failed: %v", resolveErr)),
		"Planned Actions:",
		fmt.Sprintf(" 1. %s", legacy.Command),
	}
	return lines
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
		return []string{"(none)"}
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
	reasons := []string{"Standard file synchronization"}
	if !endpoints.Destination.IsLocal {
		reasons = append(reasons, "Writing to a remote destination")
	}
	if opts.DB {
		reasons = append(reasons, "Destination database will be overwritten")
	}
	if opts.Delete {
		reasons = append(reasons, "File deletion mode enabled")
	}

	switch {
	case opts.DB || opts.Delete:
		return pterm.Red("HIGH RISK"), reasons
	case !endpoints.Destination.IsLocal:
		return pterm.Yellow("MEDIUM RISK"), reasons
	default:
		return pterm.Green("LOW RISK"), reasons
	}
}

func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
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
