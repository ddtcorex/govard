package verify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
)

// GovardVersion is the version string written into RunResult. It is set from
// the CLI at startup and falls back to a static value for tests.
var GovardVersion = "1.68.0"

var (
	ErrNeedSnapshot         = errors.New("need snapshot create (P4-08) first")
	ErrNeedAllowDestructive = errors.New("need --allow-destructive for phase 5")
)

// VerifyRunsDir returns the directory for verify JSON runs.
func VerifyRunsDir() string {
	if d := os.Getenv("GOVARD_HOME_DIR"); d != "" {
		return filepath.Join(d, "verify-runs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".govard", "verify-runs")
}

func legacyRunsDir() string {
	if d := os.Getenv("GOVARD_HOME_DIR"); d != "" {
		return filepath.Join(d, "checklist-runs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".govard", "checklist-runs")
}

// MigrateLegacyRuns copies existing checklist-runs/*.json into verify-runs on
// first use when verify-runs is empty.
func MigrateLegacyRuns() error {
	src := legacyRunsDir()
	dst := VerifyRunsDir()
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil
	}
	dstExists := false
	if _, err := os.Stat(dst); err == nil {
		if de, _ := os.ReadDir(dst); len(de) > 0 {
			dstExists = true
		}
	}
	if dstExists {
		return nil
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dst, e.Name()), b, 0644)
	}
	return nil
}

// RunResult is the JSON written per phase.
type RunResult struct {
	GovardVersion string    `json:"govard_version"`
	ProjectSHA    string    `json:"project_sha"`
	Phase         string    `json:"phase"`
	Items         []RunItem `json:"items"`
}

// RunItem is one entry in RunResult.
type RunItem struct {
	ID              string `json:"id"`
	Command         string `json:"command"`
	DurationMs      int    `json:"duration_ms"`
	ExitCode        int    `json:"exit_code"`
	Retries         int    `json:"retries"`
	EvidenceExcerpt string `json:"evidence_excerpt"`
	JSONValid       bool   `json:"json_valid"`
}

// RunPhase executes the filtered registry for a single phase and optionally
// writes the JSON file when opts.JSON is true.
func RunPhase(ctx context.Context, cfg engine.Config, phase int, opts VerifyOpts) (RunResult, error) {
	// Gate for destructive phase 5 — bypassed for --plan (dry-run).
	if phase == 5 && !opts.Plan {
		if err := checkP5Gate(); err != nil {
			return RunResult{}, err
		}
		if !opts.AllowDestructive {
			return RunResult{}, ErrNeedAllowDestructive
		}
	}

	// Filter.
	var filtered []Item
	for _, it := range Registry {
		if phase != 0 && it.Phase != phase {
			continue
		}
		if it.When != nil && !it.When(cfg) {
			continue
		}
		filtered = append(filtered, it)
	}

	// Build result.
	res := RunResult{
		GovardVersion: GovardVersion,
		ProjectSHA:    resolveProjectSHA(),
		Phase:         phaseLabel(phase),
	}

	for _, it := range filtered {
		start := time.Now()
		var ev Evidence
		if opts.Plan {
			ev = Evidence{ExitCode: 0, OutputExcerpt: "plan: " + it.Title}
		} else {
			if it.Run != nil {
				ev = it.Run(ctx, cfg, opts)
			} else {
				ev = Evidence{ExitCode: 0, OutputExcerpt: "stub"}
			}
		}
		dur := time.Since(start)
		ev.DurationMs = int(dur.Milliseconds())
		res.Items = append(res.Items, RunItem{
			ID:              it.ID,
			Command:         it.Title,
			DurationMs:      ev.DurationMs,
			ExitCode:        ev.ExitCode,
			Retries:         ev.Retries,
			EvidenceExcerpt: ev.OutputExcerpt,
			JSONValid:       ev.JSONValid,
		})
	}

	if opts.JSON {
		_ = MigrateLegacyRuns()
		dir := VerifyRunsDir()
		_ = os.MkdirAll(dir, 0755)
		ts := time.Now().Format("2006-01-02T15-04-05Z07:00")
		path := filepath.Join(dir, ts+"-phase"+phaseFileSuffix(phase)+".json")
		b, _ := json.MarshalIndent(res, "", "  ")
		_ = os.WriteFile(path, b, 0644)
	}

	return res, nil
}

func phaseLabel(phase int) string {
	if phase == 0 {
		return "all"
	}
	return "phase" + phaseFileSuffix(phase)
}

func phaseFileSuffix(phase int) string {
	if phase == 0 {
		return "all"
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimPrefix(phaseLabelRaw(phase), "phase"), " ", ""))
}

func phaseLabelRaw(phase int) string {
	switch phase {
	case 1:
		return "phase1"
	case 2:
		return "phase2"
	case 3:
		return "phase3"
	case 4:
		return "phase4"
	case 5:
		return "phase5"
	default:
		return "phase0"
	}
}

func resolveProjectSHA() string {
	// Best-effort: env override for tests, else unknown.
	if v := os.Getenv("GOVARD_VERIFY_SHA"); v != "" {
		return v
	}
	return "unknown"
}

func checkP5Gate() error {
	dir := VerifyRunsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ErrNeedSnapshot
	}
	var phase4Files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.Contains(e.Name(), "phase4") && strings.HasSuffix(e.Name(), ".json") {
			phase4Files = append(phase4Files, filepath.Join(dir, e.Name()))
		}
	}
	if len(phase4Files) == 0 {
		return ErrNeedSnapshot
	}
	sort.Strings(phase4Files)
	latest := phase4Files[len(phase4Files)-1]
	b, err := os.ReadFile(latest)
	if err != nil {
		return ErrNeedSnapshot
	}
	var res RunResult
	if err := json.Unmarshal(b, &res); err != nil {
		return ErrNeedSnapshot
	}
	for _, it := range res.Items {
		if it.ID == "P4-08" && it.ExitCode == 0 {
			return nil
		}
	}
	return ErrNeedSnapshot
}
