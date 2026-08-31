package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"govard/internal/engine"
	"govard/internal/verify"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run 5-phase Govard checklist (executable)",
	Long: `Run the 5-phase Govard verify harness (replaces manual checklist tick).

Phases:
  1 Preflight (7)  2 Bootstrap & Env (14)  3 Dev Loop (15)  4 Sync/Safety (12)  5 Destructive QA (8)

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
			return renderVerifyResult(cmd, res, jsonOut)
		}

		// All phases 1..5 sequentially. Stop on gate error.
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
			return renderVerifyResult(cmd, combined, true)
		}
		for p := 1; p <= 5; p++ {
			if p == 5 && !allowDestructive && !plan {
				pterm.Warning.Println(verify.ErrNeedAllowDestructive.Error())
				return verify.ErrNeedAllowDestructive
			}
			res, err := verify.RunPhase(ctx, cfg, p, opts)
			if err != nil {
				return err
			}
			if err := renderVerifyResult(cmd, res, false); err != nil {
				return err
			}
		}
		return nil
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
	verifyCmd.Flags().Int("phase", 0, "phase 1..5 (0=all)")
	verifyCmd.Flags().Bool("json", false, "machine JSON output")
	verifyCmd.Flags().Bool("plan", false, "dry-run (no side effects)")
	verifyCmd.Flags().Bool("allow-destructive", false, "allow phase 5 destructive")
	verifyCmd.Flags().Bool("yes", false, "alias for --allow-destructive")
	verifyCmd.Flags().Bool("allow-xdebug", false, "allow xdebug")
	verifyCmd.Flags().Int("lint-jobs", 4, "lint jobs")
	verifyCmd.Flags().String("timeout", "auto", "timeout (auto|0|<dur>)")
	verifyCmd.Flags().StringSlice("checks", nil, "checks (lint,profiler)")
	verifyCmd.Flags().String("base", "", "base ref for diff scope")
	verifyCmd.Flags().String("remote", "", "remote name")
	verifyCmd.Flags().String("project", "", "project path")
}
