package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"govard/internal/engine"
	"govard/internal/verify"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

// itemsFailedError marks a checklist run that completed with red items. It is a
// plain execution error on purpose: the process must exit 1, because 2 is
// reserved for USAGE and a failing checklist is not a usage mistake.
type itemsFailedError struct{}

func (e *itemsFailedError) Error() string { return "verify: one or more checklist items failed" }

// AlreadyReported tells the CLI that the human-facing message for this failure
// has already been written (to stderr), so `--error-json` must not append a
// second JSON document to stdout and leave two documents there.
func (e *itemsFailedError) AlreadyReported() bool { return true }

// ErrItemsFailed is returned when at least one checklist item failed.
var ErrItemsFailed = &itemsFailedError{}

// summarise turns a finished run into the process verdict. The JSON already
// carries per-item detail; this is what a CI step branches on.
//
// The human line goes to the writer the caller passes — the command's stderr,
// never stdout. Printing it through pterm wrote it to fd 1, which appended
// "checklist failed: ..." after the JSON object and broke `--json` for every
// machine consumer.
func summarise(out io.Writer, res verify.RunResult) error {
	if res.Failed() {
		passed, failed := res.Counts()
		fmt.Fprintf(out, "checklist failed: %d passed, %d failed\n", passed, failed)
		return ErrItemsFailed
	}
	return nil
}

// SummariseForTest exposes summarise for tests in /tests.
func SummariseForTest(res verify.RunResult) error { return summarise(io.Discard, res) }

// VerifyCommandForTest exposes the verify command so a test can drive it and
// inspect what lands on stdout.
func VerifyCommandForTest() *cobra.Command { return verifyCmd }

var verifyCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "verify",
	Short: "Run 5-phase Govard checklist (executable)",
	Long: `Run the 5-phase Govard verify harness (replaces manual checklist tick).

Phases:
  1 Preflight (7)  2 Bootstrap & Env (14)  3 Dev Loop (15)  4 Sync/Safety (16)  5 Destructive QA (8)

Counts are the static registry. A framework may declare extra items for its own
dev loop; RegistryFor composes them at run time, so a run can be longer.

Examples:
  govard verify --plan --json
  govard verify --phase 1 --json
  govard verify --phase 5 --allow-destructive --json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		phase, _ := cmd.Flags().GetInt("phase")
		jsonOut, _ := cmd.Flags().GetBool("json")
		plan, _ := cmd.Flags().GetBool("plan")
		allowDestructive, _ := cmd.Flags().GetBool("allow-destructive")
		allowDestructiveYes, _ := cmd.Flags().GetBool("yes")
		if allowDestructiveYes {
			allowDestructive = true
		}
		allowXdebug, _ := cmd.Flags().GetBool("allow-xdebug")
		lintJobs, _ := cmd.Flags().GetInt("lint-jobs")
		timeout, _ := cmd.Flags().GetString("timeout")
		checks, _ := cmd.Flags().GetStringSlice("checks")
		base, _ := cmd.Flags().GetString("base")
		remote, _ := cmd.Flags().GetString("remote")
		project, _ := cmd.Flags().GetString("project")

		// Resolve project root for config loading.
		root := project
		if root == "" {
			if cwd, err := os.Getwd(); err == nil {
				root = cwd
			}
		}

		var cfg engine.Config
		if root != "" {
			if loaded, _, err := engine.LoadConfigFromDir(root, false); err == nil {
				cfg = loaded
			}
			if cfg.Framework == "" {
				meta := engine.DetectFramework(root)
				cfg.Framework = meta.Framework
			}
		}

		verify.GovardVersion = Version

		opts := verify.VerifyOpts{
			Plan:             plan,
			JSON:             jsonOut,
			Remote:           remote,
			BaseRef:          base,
			Timeout:          timeout,
			Checks:           checks,
			LintJobs:         lintJobs,
			AllowDestructive: allowDestructive,
			AllowXdebug:      allowXdebug,
			ProjectRoot:      root,
		}

		// Migrate legacy runs once.
		_ = verify.MigrateLegacyRuns()

		ctx := context.Background()

		if phase != 0 {
			if phase < 1 || phase > 5 {
				return fmt.Errorf("invalid --phase %d: want 1..5", phase)
			}
			res, err := verify.RunPhase(ctx, cfg, phase, opts)
			if err != nil {
				if err == verify.ErrNeedSnapshot || err == verify.ErrNeedAllowDestructive {
					if jsonOut {
						// Still output JSON-like error for machine parsing.
						payload, _ := json.Marshal(map[string]string{"error": err.Error()})
						fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					}
					return err
				}
				return err
			}
			if err := renderVerifyResult(cmd, res, jsonOut); err != nil {
				return err
			}
			return summarise(cmd.ErrOrStderr(), res)
		}

		// All phases 1..5 sequentially. A red item does not stop the next phase —
		// the whole checklist is the deliverable, so the verdict is aggregated
		// after every phase has run.
		if jsonOut {
			var combined verify.RunResult
			var first bool
			for p := 1; p <= 5; p++ {
				if p == 5 && !allowDestructive && !plan {
					payload, _ := json.Marshal(map[string]string{"error": verify.ErrNeedAllowDestructive.Error()})
					fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return verify.ErrNeedAllowDestructive
				}
				res, err := verify.RunPhase(ctx, cfg, p, opts)
				if err != nil {
					// A gate block is reported as a JSON envelope, exactly like
					// the explicit phase-5 checks above; returning silently left
					// stdout empty for a consumer that asked for --json.
					if err == verify.ErrNeedSnapshot || err == verify.ErrNeedAllowDestructive {
						payload, _ := json.Marshal(map[string]string{"error": err.Error()})
						fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					}
					return err
				}
				if !first {
					combined = res
					combined.Phase = "all"
					first = true
				} else {
					combined.Items = append(combined.Items, res.Items...)
				}
			}
			combined.RefreshStatus()
			if err := renderVerifyResult(cmd, combined, true); err != nil {
				return err
			}
			return summarise(cmd.ErrOrStderr(), combined)
		}
		var combined verify.RunResult
		var first bool
		for p := 1; p <= 5; p++ {
			if p == 5 && !allowDestructive && !plan {
				pterm.Warning.Println(verify.ErrNeedAllowDestructive.Error())
				return verify.ErrNeedAllowDestructive
			}
			res, err := verify.RunPhase(ctx, cfg, p, opts)
			if err != nil {
				return err
			}
			if !first {
				combined = res
				combined.Phase = "all"
				first = true
			} else {
				combined.Items = append(combined.Items, res.Items...)
			}
			if err := renderVerifyResult(cmd, res, false); err != nil {
				return err
			}
		}
		combined.RefreshStatus()
		return summarise(cmd.ErrOrStderr(), combined)
	},
}

func renderVerifyResult(cmd *cobra.Command, res verify.RunResult, jsonOut bool) error {
	if jsonOut {
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal verify result: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}
	// Human table to stderr.
	pterm.Info.Printf("Phase %s: %d items\n", res.Phase, len(res.Items))
	for _, it := range res.Items {
		status := "PASS"
		if it.ExitCode != 0 {
			status = "FAIL"
		}
		pterm.Info.Printf("  %s %s (%dms) %s\n", it.ID, status, it.DurationMs, it.EvidenceExcerpt)
	}
	return nil
}

func init() {
	verifyCmd.Flags().Int("phase", 0, "Phase 1..5 (0=all)")
	verifyCmd.Flags().Bool("json", false, "Print machine-readable JSON output")
	verifyCmd.Flags().Bool("plan", false, "Dry-run (no side effects)")
	verifyCmd.Flags().Bool("allow-destructive", false, "Allow phase 5 destructive operations")
	verifyCmd.Flags().Bool("yes", false, "Alias for --allow-destructive")
	verifyCmd.Flags().Bool("allow-xdebug", false, "Allow running with Xdebug enabled")
	verifyCmd.Flags().Int("lint-jobs", 4, "Lint worker count")
	verifyCmd.Flags().String("timeout", "auto", "Timeout (auto|0|<dur>)")
	verifyCmd.Flags().StringSlice("checks", nil, "Checks (lint,profiler)")
	verifyCmd.Flags().String("base", "", "Base ref for diff scope")
	verifyCmd.Flags().String("remote", "", "Remote name")
	verifyCmd.Flags().String("project", "", "Project path")
}
