package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// ErrRunNotRecorded is returned by the all-phases CLI path when phase 4's
	// artifact could not be written and phase 5's snapshot gate therefore cannot
	// open. It wraps the write error so the operator sees the real cause.
	ErrRunNotRecorded = errors.New("phase 4 result could not be recorded")
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
	Fake          bool      `json:"fake,omitempty"`
	Items         []RunItem `json:"items"`

	// Error is set only on the document an all-phases --json run renders when
	// phase 5 refuses to start: phases 1-4 stay in Items and the gate's reason
	// rides along, so stdout is still one document. It is never written to an
	// artifact.
	Error string `json:"error,omitempty"`

	// RecordErr is why the run artifact could not be written, or nil. It is not
	// part of the artifact and never changes Status.
	RecordErr error `json:"-"`
}

// Failed reports whether any item exited non-zero.
func (r RunResult) Failed() bool {
	_, failed := r.Counts()
	return failed > 0
}

// Counts returns the number of passing and failing items. A skipped item is
// neither: it never ran, so counting it as passed would claim work that did not
// happen.
func (r RunResult) Counts() (passed, failed int) {
	for _, it := range r.Items {
		if it.Skipped {
			continue
		}
		if it.ExitCode != 0 {
			failed++
			continue
		}
		passed++
	}
	return passed, failed
}

// SkippedCount returns the number of items the run recorded as skipped. A skip
// never changes a verdict, so it is reported beside Counts, not inside it.
func (r RunResult) SkippedCount() int {
	skipped := 0
	for _, it := range r.Items {
		if it.Skipped {
			skipped++
		}
	}
	return skipped
}

// RefreshStatus recomputes Status from the items. RunPhase calls it for a single
// phase; a caller that merges phases into one result must call it again before
// rendering, or the merged artifact reports the first phase's verdict.
func (r *RunResult) RefreshStatus() {
	r.Status = "passed"
	if r.Failed() {
		r.Status = "failed"
	}
	// Fake is the top-level marker: a run containing any fake item carries it,
	// so the artifact cannot be read as a real pass. Status keeps its two
	// values for consumers that predate the field.
	r.Fake = false
	for _, it := range r.Items {
		if it.Fake {
			r.Fake = true
			break
		}
	}
}

// RunItem is one entry in RunResult. A skipped row is additive: an artifact
// written before the field existed still unmarshals, and a consumer that does
// not know about skips reads it as a passing row with exit_code 0.
type RunItem struct {
	ID              string   `json:"id"`
	Command         string   `json:"command"`
	DurationMs      int      `json:"duration_ms"`
	ExitCode        int      `json:"exit_code"`
	EvidenceExcerpt string   `json:"evidence_excerpt"`
	JSONValid       bool     `json:"json_valid"`
	Artifacts       []string `json:"artifacts,omitempty"`
	Skipped         bool     `json:"skipped,omitempty"`
	SkipReason      string   `json:"skip_reason,omitempty"`
	Fake            bool     `json:"fake,omitempty"`
}

// RunPhase executes the filtered registry for a single phase and optionally
// writes the JSON file when opts.JSON is true.
func RunPhase(ctx context.Context, cfg engine.Config, phase int, opts VerifyOpts) (RunResult, error) {
	// The destructive gate is keyed on the selection, not on phase == 5: phase 0
	// selects every phase, so it selects phase 5 too. Bypassed for --plan.
	if err := PreflightPhaseSelection([]int{phase}, opts); err != nil {
		return RunResult{}, err
	}

	// Filter. An unmet When predicate no longer drops the item: the row stays,
	// marked with the reason it did not run. A missing row is worse than a red,
	// because a red is evidence. The guard policy below marks a row the same
	// way, and so does the --checks selection.
	type filteredItem struct {
		item    Item
		skipped bool
		reason  string
	}
	var filtered []filteredItem
	for _, it := range RegistryFor(cfg) {
		if phase != 0 && it.Phase != phase {
			continue
		}
		if it.When != nil && !it.When(cfg) {
			filtered = append(filtered, filteredItem{item: it, skipped: true, reason: frameworkGateReason(it, cfg)})
			continue
		}
		// The guard policy is applied before the plan stub below, so an item
		// this run may not perform is one skipped row in plan mode too and its
		// Run is never called.
		if decision := DecideGuard(it, phase, opts); !decision.Run {
			filtered = append(filtered, filteredItem{item: it, skipped: true, reason: decision.Reason})
			continue
		}
		// The --checks selection narrows last, after the safety decision: a
		// guard verdict is a property of the run, and an item the run may not
		// perform must keep saying so even when the selection also leaves it
		// out, so the report cannot be filtered into hiding a gate. (No item
		// declares both today, so the order decides no verdict yet.)
		if len(opts.Checks) > 0 && !selectedByChecks(it.Checks, opts.Checks) {
			filtered = append(filtered, filteredItem{item: it, skipped: true, reason: checksFilterReason(opts.Checks, it.Checks)})
			continue
		}
		filtered = append(filtered, filteredItem{item: it})
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

	for _, fi := range filtered {
		it := fi.item
		start := time.Now()
		var ev Evidence
		switch {
		case fi.skipped:
			// Gated out: keep the row, do not execute the item.
			ev = Skip(fi.reason)
		case opts.Plan:
			ev = Evidence{ExitCode: 0, OutputExcerpt: "plan: " + resolveTitle(ctx, it.Title, opts)}
		default:
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
			Command:         resolveTitle(ctx, it.Title, opts),
			DurationMs:      ev.DurationMs,
			ExitCode:        ev.ExitCode,
			EvidenceExcerpt: ev.OutputExcerpt,
			JSONValid:       ev.JSONValid,
			Artifacts:       ev.Artifacts,
			Skipped:         ev.Skipped,
			SkipReason:      ev.SkipReason,
			Fake:            ev.Fake,
		})
	}

	res.RefreshStatus()

	// The artifact is always written: the phase-5 gate reads it, and --json only
	// shapes stdout. A write failure is a warning, never part of the verdict.
	if err := writeRunArtifact(res, phase, opts); err != nil {
		res.RecordErr = err
		fmt.Fprintf(os.Stderr, "warning: could not record the verify run artifact: %v\n", err)
	}

	return res, nil
}

// selectedByChecks reports whether a run that asked for requested includes an
// item that declares declared. An item that declares nothing is not
// check-specific — nothing about it belongs to one check — so it is kept
// whatever the selection is. The comparison mirrors `audit run`'s own --checks
// matching (internal/cmd/audit.go: auditChecksInclude trims the requested name
// and compares it exactly), so a name verify accepts is a name audit accepts.
func selectedByChecks(declared, requested []string) bool {
	if len(declared) == 0 {
		return true
	}
	for _, want := range requested {
		for _, have := range declared {
			if strings.TrimSpace(want) == have {
				return true
			}
		}
	}
	return false
}

// checksFilterReason is the skip a row reports when the run's --checks selection
// left the item out: the operator's own choice, named so the artifact says which
// check would have included the item.
func checksFilterReason(requested, declared []string) string {
	return "--checks " + strings.Join(requested, ",") + " excludes this item, which exercises " + strings.Join(declared, ",")
}

// frameworkGateReason is the skip a row reports when its When predicate did not
// hold. Every When in the registry is a framework predicate, so the gate that
// fired is the framework — not the item's Requires text, which on a Magento row
// reads like a prior step ("P2-01 up") and would send an operator on a Laravel
// project to debug an environment that is fine. The project's own framework is
// the one fact that explains the row, so the reason names that; which frameworks
// an item belongs to is what the phase table and the id prefixes already say.
func frameworkGateReason(it Item, cfg engine.Config) string {
	if it.WhenReason != nil {
		if reason := it.WhenReason(cfg); reason != "" {
			return reason
		}
	}
	if cfg.Framework == "" {
		return fmt.Sprintf("framework gate: %s is framework-specific and this project declares no framework", it.ID)
	}
	return fmt.Sprintf("framework gate: %s is framework-specific and this project is %s", it.ID, cfg.Framework)
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

// writeRunArtifact stores res under the project's run store.
func writeRunArtifact(res RunResult, phase int, opts VerifyOpts) error {
	_ = MigrateLegacyRuns()
	dir := ProjectRunsDir(opts.ProjectRoot)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	ts := time.Now().Format("2006-01-02T15-04-05Z07:00")
	path := filepath.Join(dir, ts+"-phase"+phaseFileSuffix(phase)+".json")
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// PreflightPhaseSelection applies the phase-5 gates to a selection of phases,
// before any item of it runs. Phase 0 means every phase and counts as phase 5.
// A selection without phase 5 is ungated, and so is a --plan run.
//
// Order: the --allow-destructive opt-in first, then the snapshot gate. The
// snapshot gate is skipped only when the selection itself contains phase 4
// and phase 5 as separate runs (the all-phases CLI path): phase 4 records the
// artifact before phase 5 starts and RunPhase(5) re-checks it then. A phase 0
// call runs both inside one RunPhase, where P5-05 reads the store and not the
// call's own result, so it needs a snapshot recorded by an earlier run.
func PreflightPhaseSelection(phases []int, opts VerifyOpts) error {
	if opts.Plan {
		return nil
	}
	has5, has4, hasAll := false, false, false
	for _, p := range phases {
		switch p {
		case 0:
			has5, hasAll = true, true
		case 4:
			has4 = true
		case 5:
			has5 = true
		}
	}
	if !has5 {
		return nil
	}
	if !opts.AllowDestructive {
		return ErrNeedAllowDestructive
	}
	if has4 && !hasAll {
		return nil
	}
	return checkP5Gate(opts)
}

// checkP5Gate requires a snapshot of this project recorded by a real (non-plan)
// phase-4 run and still present on disk. See GateSatisfyingSnapshot.
func checkP5Gate(opts VerifyOpts) error {
	if _, ok := GateSatisfyingSnapshot(opts); !ok {
		return ErrNeedSnapshot
	}
	return nil
}

// baseBranchPlaceholder is the literal P3-11 carries in its registry title.
const baseBranchPlaceholder = "{{BASE_BRANCH}}"

// resolveTitle fills the diff base into a title so plan and run output name the
// ref the row really uses. When no base resolves the row skips, and the title
// says so instead of leaking the placeholder.
func resolveTitle(ctx context.Context, title string, opts VerifyOpts) string {
	if !strings.Contains(title, baseBranchPlaceholder) {
		return title
	}
	if base, ok := ResolveDiffBase(ctx, opts.ProjectRoot, opts.BaseRef); ok {
		return strings.ReplaceAll(title, baseBranchPlaceholder, base)
	}
	return strings.ReplaceAll(title, baseBranchPlaceholder, "<no base ref found>")
}
