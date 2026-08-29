package cmd

import (
	"encoding/json"
	"fmt"
	"govard/internal/engine"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var doctorCommit bool
var doctorDryRun bool

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Aliases: []string{"diag"},
	Short:   "Run system diagnostics",
	Long: strings.TrimSpace(`
Run system diagnostics and report on the health of your local Govard environment.

Checks:
  - Docker daemon connectivity
  - Docker Compose plugin availability
  - Port conflicts on host (80/443)
  - Disk scratch write sanity
  - Govard home directory readiness (~/.govard)
  - Outbound network probe sanity
  - SSH agent connectivity and loaded keys
  - Configuration drift (framework_version, stack.php_version, db_version, node_version, search_version)

Use --fix to apply safe automatic remediations. Use --json for machine-readable output.
Use --pack to export a diagnostics support bundle for sharing with support.
When --fix is used, --dry-run shows yq-style diff without writing, and --commit git-adds and commits .govard.yml drift.
`),
	Example: `  # Run a standard diagnostic pass
  govard doctor

  # Apply safe automatic fixes when available
  govard doctor --fix

  # Export a diagnostics support pack
  govard doctor --pack

  # Trust the Govard local CA
  govard doctor trust`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outputJSON, _ := cmd.Flags().GetBool("json")
		fixEnabled, _ := cmd.Flags().GetBool("fix")
		packEnabled, _ := cmd.Flags().GetBool("pack")
		packDir, _ := cmd.Flags().GetString("pack-dir")
		doctorCommit, _ = cmd.Flags().GetBool("commit")
		doctorDryRun, _ = cmd.Flags().GetBool("dry-run")

		return ExecuteDoctor(cmd, outputJSON, fixEnabled, packEnabled, packDir, nil)
	},
}

// ExecuteDoctor runs the doctor logic. It can be invoked programmatically from other commands.
func ExecuteDoctor(cmd *cobra.Command, outputJSON bool, fixEnabled bool, packEnabled bool, packDir string, preSkipped map[string]bool) error {
	if !outputJSON {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard System Doctor ")
		fmt.Println()
	}

	report := runDoctorDiagnostics()
	if fixEnabled {
		// Multi-pass fix: Some fixes (like profile sync) can trigger others (like missing images).
		// We loop a few times if fixes were applied to try and get a clean report.
		maxPasses := 3
		skippedCheckIDs := make(map[string]bool)
		for k, v := range preSkipped {
			skippedCheckIDs[k] = v
		}
		for i := 0; i < maxPasses; i++ {
			fixResults := applyDoctorSafeFixes(report, skippedCheckIDs)
			if len(fixResults) == 0 {
				break
			}

			appliedAny := false
			for _, res := range fixResults {
				if res.Status == DoctorFixStatusApplied {
					appliedAny = true
					break
				}
			}

			if outputJSON {
				for _, line := range summarizeDoctorFixResults(fixResults) {
					fmt.Fprintln(cmd.ErrOrStderr(), line)
				}
			} else {
				renderDoctorFixResults(fixResults)
			}

			if !appliedAny {
				break // No fixes were actually applied, don't loop
			}

			// Re-run diagnostics to see what's left
			report = runDoctorDiagnostics()
		}
	}

	packPath := ""
	if packEnabled {
		cwd, _ := os.Getwd()
		resolvedPath, err := CreateDoctorDiagnosticsPack(packDir, cwd, report)
		if err != nil {
			return fmt.Errorf("create doctor diagnostics pack: %w", err)
		}
		packPath = resolvedPath
	}

	if outputJSON {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal doctor report: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
	} else {
		renderDoctorReport(report, fixEnabled)
	}
	if packPath != "" {
		if outputJSON {
			fmt.Fprintf(cmd.ErrOrStderr(), "Doctor diagnostics pack: %s\n", packPath)
		} else {
			pterm.Success.Printf("Doctor diagnostics pack: %s\n", packPath)
		}
	}

	if report.HasFailures() {
		return fmt.Errorf("doctor found %d blocking issue(s)", report.Failures)
	}
	return nil
}

var doctorTrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust the local CA for SSL certificates",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if trustCmd.RunE == nil {
			return fmt.Errorf("doctor trust is unavailable")
		}
		return trustCmd.RunE(cmd, args)
	},
}

func renderDoctorReport(report engine.DoctorReport, fixEnabled bool) {
	for _, check := range report.Checks {
		line := fmt.Sprintf("%s: %s", check.Title, check.Message)
		switch check.Status {
		case engine.DoctorStatusPass:
			pterm.Success.Println(line)
		case engine.DoctorStatusWarn:
			pterm.Warning.Println(line)
		case engine.DoctorStatusFail:
			pterm.Error.Println(line)
		default:
			pterm.Info.Println(line)
		}

		if check.Hint != "" {
			pterm.Info.Printf("Hint: %s\n", check.Hint)
		}
		if check.SuggestedCommand != "" {
			// Suppress suggestion if we just ran with --fix and it's the exact same command
			if fixEnabled && strings.Contains(check.SuggestedCommand, "doctor --fix") {
				continue
			}
			pterm.Info.Printf("Suggested next command: %s\n", check.SuggestedCommand)
		}
	}
	pterm.Info.Printf(
		"Summary: passed=%d warnings=%d failures=%d\n",
		report.Passed,
		report.Warnings,
		report.Failures,
	)
}

// DoctorCommand exposes the doctor command for tests.
func DoctorCommand() *cobra.Command {
	return doctorCmd
}

func init() {
	doctorCmd.Flags().Bool("json", false, "Print diagnostics as JSON")
	doctorCmd.Flags().Bool("fix", false, "Apply safe automatic fixes when available")
	doctorCmd.Flags().Bool("commit", false, "Commit .govard.yml drift fixes via git (implies --fix)")
	doctorCmd.Flags().Bool("dry-run", false, "Show what would be fixed without writing files (use with --fix)")
	doctorCmd.Flags().Bool("pack", false, "Export a diagnostics support pack")
	doctorCmd.Flags().String("pack-dir", "", "Output directory for diagnostics pack (default: ~/.govard/diagnostics)")
	doctorCmd.AddCommand(doctorTrustCmd)
}
