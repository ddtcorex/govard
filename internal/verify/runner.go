package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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

// VerifyRunsDir returns the root directory for verify JSON runs. Run artifacts
// live one level down, under ProjectRunsDir, so one project's evidence can never
// satisfy another project's gate (issue #462).
func VerifyRunsDir() string {
	if d := os.Getenv("GOVARD_HOME_DIR"); d != "" {
		return filepath.Join(d, "verify-runs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".govard", "verify-runs")
}

// ProjectID keys the local run store to a project. The canonical working-tree
// path is the whole identity on purpose: run artifacts describe one checkout,
// and a moved checkout is a different local store (the same trade-off the audit
// store and DSH project memory already make).
func ProjectID(projectRoot string) string {
	canonical := projectRoot
	if resolved, err := resolveCanonicalRoot(projectRoot); err == nil && resolved != "" {
		canonical = resolved
	}
	sum := sha256.Sum256([]byte(canonical))
	return "project-" + hex.EncodeToString(sum[:])[:16]
}

// resolveCanonicalRoot absolutises and dereferences a project root. An empty
// root means the current directory, matching how cmd/verify.go resolves it. A
// path that does not exist still yields a stable cleaned value so ProjectID
// never fails on a root that is merely absent.
func resolveCanonicalRoot(projectRoot string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		projectRoot = cwd
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return filepath.Clean(abs), nil
}

// ProjectRunsDir is the per-project run store: <store root>/<project id>.
func ProjectRunsDir(projectRoot string) string {
	return filepath.Join(VerifyRunsDir(), ProjectID(projectRoot))
}

// ResolveProjectSHA returns the revision the run describes, or "unknown" when
// the root is not a git repository. GOVARD_VERIFY_SHA wins so a CI job can pin
// the value it built.
func ResolveProjectSHA(projectRoot string) string {
	if v := strings.TrimSpace(os.Getenv("GOVARD_VERIFY_SHA")); v != "" {
		return v
	}
	root, err := resolveCanonicalRoot(projectRoot)
	if err != nil {
		return "unknown"
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	if sha := strings.TrimSpace(string(out)); sha != "" {
		return sha
	}
	return "unknown"
}

// ResolveProjectSHAForTest exposes ResolveProjectSHA for tests in /tests.
func ResolveProjectSHAForTest(projectRoot string) string { return ResolveProjectSHA(projectRoot) }

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
	ProjectID     string    `json:"project_id,omitempty"`
	Phase         string    `json:"phase"`
	Mode          string    `json:"mode,omitempty"`
	Status        string    `json:"status,omitempty"`
	Items         []RunItem `json:"items"`
}

// Failed reports whether any item exited non-zero.
func (r RunResult) Failed() bool {
	_, failed := r.Counts()
	return failed > 0
}

// Counts returns the number of passing and failing items.
func (r RunResult) Counts() (passed, failed int) {
	for _, it := range r.Items {
		if it.ExitCode != 0 {
			failed++
			continue
		}
		passed++
	}
	return passed, failed
}

// RefreshStatus recomputes Status from the items. RunPhase calls it for a single
// phase; a caller that merges phases into one result must call it again before
// rendering, or the merged artifact reports the first phase's verdict.
func (r *RunResult) RefreshStatus() {
	r.Status = "passed"
	if r.Failed() {
		r.Status = "failed"
	}
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
		ProjectSHA:    ResolveProjectSHA(opts.ProjectRoot),
		ProjectID:     ProjectID(opts.ProjectRoot),
		Phase:         phaseLabel(phase),
		Mode:          "run",
	}
	if opts.Plan {
		res.Mode = "plan"
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

	res.RefreshStatus()

	if opts.JSON {
		_ = MigrateLegacyRuns()
		dir := ProjectRunsDir(opts.ProjectRoot)
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
