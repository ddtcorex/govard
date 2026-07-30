package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

func resolveMediaModeFlagValue(cmd *cobra.Command, current string, args []string) string {
	if cmd == nil || !cmd.Flags().Changed("media") {
		return current
	}
	if normalized, ok := normalizeExplicitMediaModeArg(args); ok {
		return normalized
	}
	return current
}

func normalizeExplicitMediaModeArg(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	candidate := strings.ToLower(strings.TrimSpace(args[0]))
	switch candidate {
	case MediaSyncNone, MediaSyncMinimal, MediaSyncOptimized, MediaSyncAll, "catalog":
		return candidate, true
	default:
		return "", false
	}
}

func resolvePositionalPathArg(cmd *cobra.Command, currentPath string, args []string) string {
	if cmd == nil || cmd.Flags().Changed("path") || currentPath != "" {
		return currentPath
	}
	if len(args) == 0 {
		return currentPath
	}
	candidate := strings.TrimSpace(args[0])
	if candidate == "" {
		return currentPath
	}
	if _, consumedByMedia := normalizeExplicitMediaModeArg(args); consumedByMedia {
		return currentPath
	}
	return candidate
}

// ResolvePositionalPathArgForTest exposes resolvePositionalPathArg's precedence
// rules for testing without needing a full cobra.Command. pathFlagChanged
// simulates cmd.Flags().Changed("path").
func ResolvePositionalPathArgForTest(pathFlagChanged bool, currentPath string, args []string) string {
	if pathFlagChanged || currentPath != "" {
		return currentPath
	}
	if len(args) == 0 {
		return currentPath
	}
	candidate := strings.TrimSpace(args[0])
	if candidate == "" {
		return currentPath
	}
	if _, consumedByMedia := normalizeExplicitMediaModeArg(args); consumedByMedia {
		return currentPath
	}
	return candidate
}
