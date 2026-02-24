package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"govard/internal/engine"

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
	DBType           string `json:"db_type"`
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
	Short: "Show recommended runtime profile for the detected framework",
	RunE: func(cmd *cobra.Command, args []string) error {
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

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Detected framework: %s\n", metadata.Framework)
		if metadata.Version != "" {
			fmt.Fprintf(out, "Detected framework version: %s\n", metadata.Version)
		} else {
			fmt.Fprintln(out, "Detected framework version: (not detected)")
		}
		fmt.Fprintf(out, "Profile source: %s\n", result.Source)
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Recommended Govard profile:")
		fmt.Fprintf(out, "  recipe: %s\n", result.Profile.Framework)
		if result.Profile.FrameworkVersion != "" {
			fmt.Fprintf(out, "  framework_version: %s\n", result.Profile.FrameworkVersion)
		}
		fmt.Fprintf(out, "  stack.php_version: %s\n", result.Profile.PHPVersion)
		if result.Profile.NodeVersion != "" {
			fmt.Fprintf(out, "  stack.node_version: %s\n", result.Profile.NodeVersion)
		}
		fmt.Fprintf(out, "  stack.db_type: %s\n", result.Profile.DBType)
		if result.Profile.DBVersion != "" {
			fmt.Fprintf(out, "  stack.db_version: %s\n", result.Profile.DBVersion)
		}
		if result.Profile.WebRoot != "" {
			fmt.Fprintf(out, "  stack.web_root: %s\n", result.Profile.WebRoot)
		}
		fmt.Fprintf(out, "  stack.services.web_server: %s\n", result.Profile.WebServer)
		fmt.Fprintf(out, "  stack.services.cache: %s\n", result.Profile.Cache)
		if result.Profile.CacheVersion != "" {
			fmt.Fprintf(out, "  stack.cache_version: %s\n", result.Profile.CacheVersion)
		}
		fmt.Fprintf(out, "  stack.services.search: %s\n", result.Profile.Search)
		if result.Profile.SearchVersion != "" {
			fmt.Fprintf(out, "  stack.search_version: %s\n", result.Profile.SearchVersion)
		}
		fmt.Fprintf(out, "  stack.services.queue: %s\n", result.Profile.Queue)
		if result.Profile.QueueVersion != "" {
			fmt.Fprintf(out, "  stack.queue_version: %s\n", result.Profile.QueueVersion)
		}

		if len(result.Notes) > 0 {
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Notes:")
			for _, note := range result.Notes {
				fmt.Fprintf(out, "  - %s\n", note)
			}
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Warnings:")
			for _, warning := range result.Warnings {
				fmt.Fprintf(out, "  - %s\n", warning)
			}
		}

		return nil
	},
}

var profileApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the recommended runtime profile to govard.yml",
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
		engine.NormalizeConfig(&config)
		saveConfig(config)
		pterm.Success.Println("Applied profile to govard.yml")
		return nil
	},
}

func init() {
	registerProfileFlags(profileCmd)
	profileCmd.AddCommand(profileApplyCmd)
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
		metadata.Framework = strings.ToLower(v)
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
			DBType:           result.Profile.DBType,
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
