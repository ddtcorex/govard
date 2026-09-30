package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var remoteAuditCmd = &cobra.Command{
	// `stats` and `tail` only read the local remote audit log, so they need
	// neither ssh nor rsync, which the parent `remote` group declares.
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "audit",
	Short: "Inspect remote operation audit log",
}

var remoteAuditTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Print recent remote audit events",
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, _ := cmd.Flags().GetInt("lines")
		statusFilter, _ := cmd.Flags().GetString("status")
		operationFilter, _ := cmd.Flags().GetString("operation")
		sinceRaw, _ := cmd.Flags().GetString("since")
		untilRaw, _ := cmd.Flags().GetString("until")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		since, until, err := parseAuditTimeFilters(sinceRaw, untilRaw)
		if err != nil {
			return err
		}

		events, err := remote.ReadAuditEvents(lines)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				pterm.Info.Printf("No remote audit log found at %s\n", remote.AuditLogPath())
				return nil
			}
			return fmt.Errorf("read remote audit log: %w", err)
		}

		filtered := filterAuditEvents(events, statusFilter, operationFilter, since, until)
		if len(filtered) == 0 {
			pterm.Info.Println("No matching remote audit events.")
			return nil
		}

		if jsonOutput {
			payload, err := json.MarshalIndent(filtered, "", "  ")
			if err != nil {
				return fmt.Errorf("encode audit events as JSON: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return nil
		}

		for _, event := range filtered {
			duration := ""
			if event.DurationMS > 0 {
				duration = fmt.Sprintf(" (%dms)", event.DurationMS)
			}
			details := []string{}
			if event.Remote != "" {
				details = append(details, "remote="+event.Remote)
			}
			if event.Source != "" || event.Destination != "" {
				details = append(details, fmt.Sprintf("flow=%s->%s", displayFlowNode(event.Source), displayFlowNode(event.Destination)))
			}
			if event.Category != "" {
				details = append(details, "category="+event.Category)
			}
			meta := ""
			if len(details) > 0 {
				meta = " [" + strings.Join(details, " ") + "]"
			}
			message := strings.TrimSpace(event.Message)
			if message == "" {
				message = "-"
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"%s %-7s %-18s%s%s %s\n",
				event.Timestamp,
				strings.ToUpper(event.Status),
				event.Operation,
				duration,
				meta,
				message,
			)
		}
		return nil
	},
}

var remoteAuditStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Summarize remote audit events",
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, _ := cmd.Flags().GetInt("lines")
		statusFilter, _ := cmd.Flags().GetString("status")
		operationFilter, _ := cmd.Flags().GetString("operation")
		sinceRaw, _ := cmd.Flags().GetString("since")
		untilRaw, _ := cmd.Flags().GetString("until")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		since, until, err := parseAuditTimeFilters(sinceRaw, untilRaw)
		if err != nil {
			return err
		}

		events, err := remote.ReadAuditEvents(lines)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				pterm.Info.Printf("No remote audit log found at %s\n", remote.AuditLogPath())
				return nil
			}
			return fmt.Errorf("read remote audit log: %w", err)
		}

		filtered := filterAuditEvents(events, statusFilter, operationFilter, since, until)
		if len(filtered) == 0 {
			pterm.Info.Println("No matching remote audit events.")
			return nil
		}

		stats := computeAuditStats(filtered)
		if jsonOutput {
			payload, err := json.MarshalIndent(stats, "", "  ")
			if err != nil {
				return fmt.Errorf("encode audit stats as JSON: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return nil
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Remote Audit Stats")
		fmt.Fprintf(cmd.OutOrStdout(), "events: %d\n", stats.Total)
		printCounterSection(cmd, "status", stats.ByStatus)
		printCounterSection(cmd, "category", stats.ByCategory)
		printCounterSection(cmd, "operation", stats.ByOperation)
		return nil
	},
}

func init() {
	remoteAuditTailCmd.Flags().Int("lines", 20, "Number of recent audit events to load")
	remoteAuditTailCmd.Flags().String("status", "", "Filter by status (success, failure, warning, plan)")
	remoteAuditTailCmd.Flags().String("operation", "", "Filter by operation name (substring match)")
	remoteAuditTailCmd.Flags().String("since", "", "Include events at or after time (RFC3339 or YYYY-MM-DD)")
	remoteAuditTailCmd.Flags().String("until", "", "Include events at or before time (RFC3339 or YYYY-MM-DD)")
	remoteAuditTailCmd.Flags().Bool("json", false, "Print filtered events as JSON")

	remoteAuditStatsCmd.Flags().Int("lines", 200, "Number of recent audit events to load")
	remoteAuditStatsCmd.Flags().String("status", "", "Filter by status (success, failure, warning, plan)")
	remoteAuditStatsCmd.Flags().String("operation", "", "Filter by operation name (substring match)")
	remoteAuditStatsCmd.Flags().String("since", "", "Include events at or after time (RFC3339 or YYYY-MM-DD)")
	remoteAuditStatsCmd.Flags().String("until", "", "Include events at or before time (RFC3339 or YYYY-MM-DD)")
	remoteAuditStatsCmd.Flags().Bool("json", false, "Print stats as JSON")

	remoteAuditCmd.AddCommand(remoteAuditTailCmd)
	remoteAuditCmd.AddCommand(remoteAuditStatsCmd)
}

func filterAuditEvents(events []remote.AuditEvent, statusFilter string, operationFilter string, since *time.Time, until *time.Time) []remote.AuditEvent {
	statusFilter = strings.ToLower(strings.TrimSpace(statusFilter))
	operationFilter = strings.ToLower(strings.TrimSpace(operationFilter))
	filtered := make([]remote.AuditEvent, 0, len(events))
	for _, event := range events {
		if statusFilter != "" && strings.ToLower(event.Status) != statusFilter {
			continue
		}
		if operationFilter != "" && !strings.Contains(strings.ToLower(event.Operation), operationFilter) {
			continue
		}
		if since != nil || until != nil {
			ts, ok := parseEventTimestamp(event.Timestamp)
			if !ok {
				continue
			}
			if since != nil && ts.Before(*since) {
				continue
			}
			if until != nil && ts.After(*until) {
				continue
			}
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func displayFlowNode(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

type auditStats struct {
	Total       int            `json:"total"`
	ByStatus    map[string]int `json:"by_status"`
	ByCategory  map[string]int `json:"by_category"`
	ByOperation map[string]int `json:"by_operation"`
}

func computeAuditStats(events []remote.AuditEvent) auditStats {
	stats := auditStats{
		Total:       len(events),
		ByStatus:    map[string]int{},
		ByCategory:  map[string]int{},
		ByOperation: map[string]int{},
	}
	for _, event := range events {
		status := strings.ToLower(strings.TrimSpace(event.Status))
		if status == "" {
			status = "unknown"
		}
		stats.ByStatus[status]++

		category := strings.ToLower(strings.TrimSpace(event.Category))
		if category == "" {
			category = "none"
		}
		stats.ByCategory[category]++

		operation := strings.ToLower(strings.TrimSpace(event.Operation))
		if operation == "" {
			operation = "unknown"
		}
		stats.ByOperation[operation]++
	}
	return stats
}

type counterEntry struct {
	Key   string
	Count int
}

func sortedCounters(counter map[string]int) []counterEntry {
	entries := make([]counterEntry, 0, len(counter))
	for key, count := range counter {
		entries = append(entries, counterEntry{Key: key, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Count > entries[j].Count
	})
	return entries
}

func printCounterSection(cmd *cobra.Command, title string, counter map[string]int) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title)
	entries := sortedCounters(counter)
	for _, entry := range entries {
		fmt.Fprintf(cmd.OutOrStdout(), " - %s: %d\n", entry.Key, entry.Count)
	}
}

func parseAuditTimeFilters(sinceRaw string, untilRaw string) (*time.Time, *time.Time, error) {
	var since *time.Time
	var until *time.Time

	if strings.TrimSpace(sinceRaw) != "" {
		parsed, err := parseAuditTimeFilter(sinceRaw, false)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --since: %w", err)
		}
		since = &parsed
	}
	if strings.TrimSpace(untilRaw) != "" {
		parsed, err := parseAuditTimeFilter(untilRaw, true)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --until: %w", err)
		}
		until = &parsed
	}
	if since != nil && until != nil && since.After(*until) {
		return nil, nil, fmt.Errorf("--since must be earlier than or equal to --until")
	}
	return since, until, nil
}

func parseAuditTimeFilter(value string, isUpperBound bool) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty time filter")
	}

	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		if isUpperBound {
			return parsed.Add(24*time.Hour - time.Nanosecond), nil
		}
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD, got %q", trimmed)
}

func parseEventTimestamp(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}
