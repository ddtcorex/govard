package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	profileFrameworkOverride string
	profileVersionOverride   string
	profileJSONOutput        bool
)

type profileDetectedPayload struct {
	Framework string `json:"framework"`
	Version   string `json:"version"`
}

type profileSelectedPayload struct {
	Framework        string `json:"framework"`
	FrameworkVersion string `json:"framework_version"`
	PHPVersion       string `json:"php_version"`
	NodeVersion      string `json:"node_version"`
	DB               string `json:"db"`
	DBVersion        string `json:"db_version"`
	WebRoot          string `json:"web_root"`
	WebServer        string `json:"web_server"`
	Cache            string `json:"cache"`
	CacheVersion     string `json:"cache_version"`
	Search           string `json:"search"`
	SearchVersion    string `json:"search_version"`
	Queue            string `json:"queue"`
	QueueVersion     string `json:"queue_version"`
	XdebugSession    string `json:"xdebug_session"`
}

type profileOutputPayload struct {
	Detected profileDetectedPayload `json:"detected"`
	Selected profileSelectedPayload `json:"selected"`
	Source   string                 `json:"source"`
	Notes    []string               `json:"notes"`
	Warnings []string               `json:"warnings"`
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage environment profiles (show, switch, apply, clear)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		out := cmd.OutOrStdout()

		// Header with active profile
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Profile Status ")
		fmt.Println()

		// Show current active profile from registry
		if entry, ok := engine.GetProjectRegistryEntry(cwd); ok {
			if entry.Profile != "" {
				fmt.Fprintf(out, "Active: %s\n", pterm.Cyan(entry.Profile))
			} else {
				fmt.Fprintf(out, "Active: %s\n", pterm.Gray("(default)"))
			}
		}

		metadata, result, err := resolveProfileForCurrentProject()
		if err != nil {
			return err
		}

		if profileJSONOutput {
			payload := buildProfileOutputPayload(metadata, result)
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(payload)
		}

		fmt.Fprintf(out, "Framework: %s", pterm.Magenta(metadata.Framework))
		if metadata.Version != "" {
			fmt.Fprintf(out, " (%s)", metadata.Version)
		}
		fmt.Fprintln(out, "")

		pterm.DefaultSection.WithLevel(2).Println("Recommended Profile")

		tableData := pterm.TableData{
			{"Setting", "Value"},
		}

		addRow := func(label, value string) {
			tableData = append(tableData, []string{label, value})
		}

		addRow("Framework", result.Profile.Framework)
		if result.Profile.FrameworkVersion != "" {
			addRow("Framework Version", result.Profile.FrameworkVersion)
		}
		addRow("PHP Version", result.Profile.PHPVersion)
		if result.Profile.NodeVersion != "" {
			addRow("Node Version", result.Profile.NodeVersion)
		}
		addRow("Database", result.Profile.DB)
		if result.Profile.DBVersion != "" {
			addRow("DB Version", result.Profile.DBVersion)
		}
		if result.Profile.WebRoot != "" {
			addRow("Web Root", result.Profile.WebRoot)
		}
		addRow("Web Server", result.Profile.WebServer)
		addRow("Cache", result.Profile.Cache)
		if result.Profile.CacheVersion != "" {
			addRow("Cache Version", result.Profile.CacheVersion)
		}
		addRow("Search", result.Profile.Search)
		if result.Profile.SearchVersion != "" {
			addRow("Search Version", result.Profile.SearchVersion)
		}
		addRow("Queue", result.Profile.Queue)
		if result.Profile.QueueVersion != "" {
			addRow("Queue Version", result.Profile.QueueVersion)
		}

		_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

		if len(result.Notes) > 0 {
			fmt.Fprintln(out, "")
			pterm.Warning.Println("Notes:")
			for _, note := range result.Notes {
				fmt.Fprintf(out, "  • %s\n", note)
			}
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintln(out, "")
			pterm.Warning.Println("Warnings:")
			for _, warning := range result.Warnings {
				fmt.Fprintf(out, "  • %s\n", warning)
			}
		}

		return nil
	},
}

var profileApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the recommended runtime profile to .govard.yml",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, result, err := resolveProfileForCurrentProject()
		if err != nil {
			return err
		}

		wd, _ := os.Getwd()
		config, err := engine.LoadBaseConfigFromDir(wd, false)
		if err != nil {
			return err
		}

		engine.ApplyRuntimeProfileToConfig(&config, result.Profile)
		engine.NormalizeConfig(&config, wd)
		saveConfig(config)
		pterm.Success.Println("Applied profile to .govard.yml")
		return nil
	},
}

var profileSwitchCmd = &cobra.Command{
	Use:   "switch [profile_name]",
	Short: "Switch to a different environment profile",
	Long: `Switches the active environment profile for the current project.
The profile name is persisted per-project in ~/.govard/projects.json.

When called without an argument, shows an interactive profile selector.
When called with a profile name, switches directly.

Note: After switching, run 'govard env up' to apply the new environment.
Use 'govard config profile clear' to reset to default profile.`,
	Example: `  # Interactive profile selection
  govard config profile switch

  # Switch directly to a profile
  govard config profile switch upgrade`,
	RunE: runProfileSwitch,
}

var profileClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Reset to default profile (clears saved profile)",
	Long:  `Clears the saved profile, reverting to the default (no profile) behavior.`,
	RunE:  runProfileClear,
}

func init() {
	registerProfileFlags(profileCmd)
	profileCmd.AddCommand(profileApplyCmd)
	profileCmd.AddCommand(profileSwitchCmd)
	profileCmd.AddCommand(profileClearCmd)
	configCmd.AddCommand(profileCmd)
}

func registerProfileFlags(command *cobra.Command) {
	command.PersistentFlags().StringVar(&profileFrameworkOverride, "framework", "", "Override detected framework")
	command.PersistentFlags().StringVar(&profileVersionOverride, "framework-version", "", "Override detected framework version")
	command.PersistentFlags().BoolVar(&profileJSONOutput, "json", false, "Output selected profile as JSON")
}

func resolveProfileForCurrentProject() (engine.ProjectMetadata, engine.RuntimeProfileResult, error) {
	cwd, _ := os.Getwd()
	metadata := engine.DetectFramework(cwd)

	if v := strings.TrimSpace(profileFrameworkOverride); v != "" {
		metadata.Framework = frameworks.Normalize(v)
	}
	if v := strings.TrimSpace(profileVersionOverride); v != "" {
		metadata.Version = v
	}

	if metadata.Framework == "" || metadata.Framework == "generic" {
		return metadata, engine.RuntimeProfileResult{}, fmt.Errorf("could not detect framework, please use --framework")
	}

	result, err := engine.ResolveRuntimeProfile(metadata.Framework, metadata.Version)
	if err != nil {
		return metadata, engine.RuntimeProfileResult{}, err
	}

	return metadata, result, nil
}

func buildProfileOutputPayload(metadata engine.ProjectMetadata, result engine.RuntimeProfileResult) profileOutputPayload {
	notes := result.Notes
	if notes == nil {
		notes = []string{}
	}
	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return profileOutputPayload{
		Detected: profileDetectedPayload{
			Framework: metadata.Framework,
			Version:   metadata.Version,
		},
		Selected: profileSelectedPayload{
			Framework:        result.Profile.Framework,
			FrameworkVersion: result.Profile.FrameworkVersion,
			PHPVersion:       result.Profile.PHPVersion,
			NodeVersion:      result.Profile.NodeVersion,
			DB:               result.Profile.DB,
			DBVersion:        result.Profile.DBVersion,
			WebRoot:          result.Profile.WebRoot,
			WebServer:        result.Profile.WebServer,
			Cache:            result.Profile.Cache,
			CacheVersion:     result.Profile.CacheVersion,
			Search:           result.Profile.Search,
			SearchVersion:    result.Profile.SearchVersion,
			Queue:            result.Profile.Queue,
			QueueVersion:     result.Profile.QueueVersion,
			XdebugSession:    result.Profile.XdebugSession,
		},
		Source:   result.Source,
		Notes:    notes,
		Warnings: warnings,
	}
}

// detectAvailableProfiles finds all .govard.<name>.yml files in the project root.
func detectAvailableProfiles(projectRoot string) []string {
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return nil
	}

	var profiles []string
	prefix := ".govard."
	suffix := ".yml"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) && len(name) > len(prefix+suffix) {
			profile := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
			// Skip special files
			if profile == "local" || profile == "project.local" || profile == "compose" {
				continue
			}
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

// runProfileSwitch handles the profile switch command.
func runProfileSwitch(cmd *cobra.Command, args []string) error {
	cwd, _ := os.Getwd()

	// 1. Determine profile name
	var profileName string
	if len(args) > 0 {
		profileName = args[0]
		// Handle "default" as empty profile
		if profileName == "default" {
			profileName = ""
		}
	} else {
		// Interactive mode - add "None (default)" option
		profiles := detectAvailableProfiles(cwd)
		if len(profiles) == 0 {
			pterm.Warning.Println("No profile files (.govard.<name>.yml) found in this project.")
			pterm.Info.Println("Use 'govard config profile clear' to reset to default.")
			return nil
		}

		// Add "None (default)" option at the beginning
		options := append([]string{"None (default)"}, profiles...)

		selected, err := pterm.DefaultInteractiveSelect.
			WithOptions(options).
			Show("Select a profile to switch to")
		if err != nil || selected == "" {
			return fmt.Errorf("profile selection cancelled")
		}
		if selected == "None (default)" {
			profileName = ""
		} else {
			profileName = selected
		}
	}

	// Validate profile exists (empty for default is allowed)
	if profileName != "" {
		profilePath := fmt.Sprintf("%s/.govard.%s.yml", cwd, profileName)
		if _, err := os.Stat(profilePath); os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found (expected file: %s)", profileName, profilePath)
		}
	} else {
		pterm.Info.Println("Switching to default profile (no profile)")
	}

	// 2. Update project registry with new profile
	// Empty profileName means "use default" - clear the saved profile
	// Save current profile as previous_profile for shift detection
	var previousProfile string
	if existingEntry, ok := engine.GetProjectRegistryEntry(cwd); ok {
		previousProfile = existingEntry.Profile
	}
	entry := engine.ProjectRegistryEntry{
		Path:            cwd,
		Profile:         profileName,
		PreviousProfile: previousProfile, // Store old profile for shift detection
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		return fmt.Errorf("save profile to registry: %w", err)
	}

	// Note: Don't update .govard.lock here - let env up handle it after detecting shift
	// This ensures DetectProfileShift can compare old vs new profile correctly

	// Print success message
	if profileName == "" {
		pterm.Success.Println("Switched to default profile (no profile)")
	} else {
		pterm.Success.Printf("Switched to profile %q\n", profileName)
	}

	// Warn user to restart environment if profile actually changed
	// Show warning when switching to/from default (empty) profile, or between profiles
	if previousProfile != profileName {
		fmt.Println()
		pterm.Warning.Println("Profile changed. Run 'govard env up' to apply the new environment.")
	}
	return nil
}

func runProfileClear(cmd *cobra.Command, args []string) error {
	cwd, _ := os.Getwd()

	// Save current profile as previous_profile for shift detection
	var previousProfile string
	if existingEntry, ok := engine.GetProjectRegistryEntry(cwd); ok {
		previousProfile = existingEntry.Profile
	}

	// Clear profile from project registry (including previous_profile)
	entry := engine.ProjectRegistryEntry{
		Path:            cwd,
		Profile:         "",              // Empty = default profile
		PreviousProfile: previousProfile, // Store old profile for shift detection
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		return fmt.Errorf("clear profile from registry: %w", err)
	}

	pterm.Success.Println("Profile reset to default")

	// Warn user to restart environment
	fmt.Println()
	pterm.Warning.Println("Profile changed. Run 'govard env up' to apply the new environment.")
	return nil
}
