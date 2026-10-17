package desktop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
)

// DoctorService backend (Task 1, WS-E health UX): wraps the existing doctor
// engine with a per-check fix allowlist so dead warnings become one-click
// fixes. Safe fixes apply directly; risky fixes (network, volume, config
// mutations, and anything touching a running project container) need a
// dry-run preview token plus an explicit confirm; checks with no safe fix
// expose fixKind none and no fix path at all.
//
// Task 3 binds these to the frontend (DoctorService methods delegating to
// these functions plus bridge.js routes); this file stays binding-free so
// the bindings contract tests are unaffected until then.
//
// Project scoping: unscoped calls resolve project files from the process
// working directory, exactly like `govard doctor --fix` run from a project
// root. The ForProject variants scope the real dispatcher to an explicit
// project root instead (Task 3 wiring); the allowlist, token, and sanitize
// layers here are root-agnostic.

type DoctorFixKind string

const (
	DoctorFixKindSafe  DoctorFixKind = "safe"
	DoctorFixKindRisky DoctorFixKind = "risky"
	DoctorFixKindNone  DoctorFixKind = "none"
)

// DoctorCheckView is one engine check plus its fix affordance for the UI.
type DoctorCheckView struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Status           string        `json:"status"`
	Message          string        `json:"message"`
	Hint             string        `json:"hint,omitempty"`
	SuggestedCommand string        `json:"suggestedCommand,omitempty"`
	FixKind          DoctorFixKind `json:"fixKind"`
	FixLabel         string        `json:"fixLabel,omitempty"`
	NeedsPreview     bool          `json:"needsPreview"`
}

// DoctorReportView is the sanitized run result the health header renders.
type DoctorReportView struct {
	Checks   []DoctorCheckView `json:"checks"`
	Passed   int               `json:"passed"`
	Warnings int               `json:"warnings"`
	Failures int               `json:"failures"`
	OpID     string            `json:"opId"`
}

// DoctorFixPreview is the dry-run plan for a risky fix. PreviewID is a
// single-use token bound to CheckID; ApplyDoctorFix consumes it.
type DoctorFixPreview struct {
	PreviewID       string   `json:"previewId"`
	CheckID         string   `json:"checkId"`
	Summary         string   `json:"summary"`
	Actions         []string `json:"actions,omitempty"`
	RequiresConfirm bool     `json:"requiresConfirm"`
}

// DoctorFixOutcome is the per-check result of one applied fix.
type DoctorFixOutcome struct {
	CheckID string   `json:"checkId"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Actions []string `json:"actions,omitempty"`
}

// DoctorFixOutcomeApplied is the success status for DoctorFixOutcome.Status.
const DoctorFixOutcomeApplied = "applied"

// DoctorFixExecutor runs (dryRun=false) or plans (dryRun=true, no
// mutations) the fix for one allowlisted check. Implementations must honor
// ctx cancellation for long operations (image pulls) and must not mutate
// anything when dryRun is true.
type DoctorFixExecutor func(ctx context.Context, checkID string, dryRun bool) (actions []string, message string, err error)

// UI text caps: engine and executor strings can be arbitrarily long (image
// lists, drift details); the views stay toast-sized.
const (
	doctorUITitleMax   = 200
	doctorUIMessageMax = 500
	doctorUIActionMax  = 280
)

// doctorSafeFixes is the 1-click allowlist: permission repairs and prunes
// that never touch a running container, the network, or project config.
var doctorSafeFixes = map[string]bool{
	"host.govard.home":     true,
	"host.compose.spam":    true,
	"host.govard.registry": true,
}

// doctorRiskyFixes need a dry-run preview token plus explicit confirm:
// service mutations on running containers, network/volume work, and
// project config writes. In particular host.search.index_block issues a
// write into the running search container, so per the plan review focus it
// is never 1-click even though it reads like a "service restart".
var doctorRiskyFixes = map[string]bool{
	"host.search.index_block": true,
	"project.runtime.images":  true,
	"project.config.audit":    true,
	"project.config.drift":    true,
	"project.profile.sync":    true,
}

// doctorKnownNoFix pins the engine checks that have no fix path, so a new
// engine check ID fails closed ("unknown doctor check") instead of
// silently landing on none: the allowlist reviewer must classify it.
var doctorKnownNoFix = map[string]bool{
	"docker.daemon":         true,
	"docker.compose":        true,
	"host.port.80":          true,
	"host.port.443":         true,
	"host.disk.scratch":     true,
	"host.network.outbound": true,
	"host.ssh.agent":        true,
	"host.deps.docker":      true,
	"host.deps.remote":      true,
}

var doctorRunDiagnosticsForDesktop = func() engine.DoctorReport {
	return engine.RunDoctorDiagnostics(engine.DoctorDependencies{})
}

// doctorRunDiagnosticsForProjectForDesktop is the project-scoped doctor
// engine run behind RunDoctorForProject: the project-bound checks read from
// the given root instead of the process working directory.
var doctorRunDiagnosticsForProjectForDesktop = func(projectRoot string) engine.DoctorReport {
	return engine.RunDoctorDiagnosticsInRoot(engine.DoctorDependencies{}, projectRoot)
}

var doctorFixExecutorForDesktop DoctorFixExecutor = defaultDoctorFixExecutor

// defaultDoctorFixExecutor is the production per-check fix executor used
// when tests do not override the seam.
var defaultDoctorFixExecutor DoctorFixExecutor = dispatchDoctorFix

// doctorOpSequence makes registry op ids unique across runs and applies.
var doctorOpSequence atomic.Uint64

// doctorRandRead is rand.Read behind a seam so preview-token minting
// failures are testable without starving the host of entropy.
var doctorRandRead = rand.Read

func nextDoctorOpID(prefix string) string {
	return prefix + ":" + strconv.FormatUint(doctorOpSequence.Add(1), 10)
}

// doctorPreviewToken is a single-use grant to apply one risky fix for the
// exact plan it was minted for: planHash binds the dry-run content so state
// that drifts between preview and apply cannot ride an old token.
type doctorPreviewToken struct {
	checkID   string
	planHash  string
	createdAt time.Time
}

// doctorPreviewTokenTTL bounds how long a dry-run plan stays appliable:
// config and container state drift, so a stale plan must not apply.
const doctorPreviewTokenTTL = 15 * time.Minute

var doctorPreviewTokens = struct {
	sync.Mutex
	byID map[string]doctorPreviewToken
}{byID: map[string]doctorPreviewToken{}}

func resetDoctorPreviewTokens() {
	doctorPreviewTokens.Lock()
	defer doctorPreviewTokens.Unlock()
	doctorPreviewTokens.byID = map[string]doctorPreviewToken{}
}

func mintDoctorPreviewToken(checkID string, planHash string) (string, error) {
	var raw [16]byte
	if _, err := doctorRandRead(raw[:]); err != nil {
		// No randomness, no token: a fallback token this store never saw
		// would be unusable, and returning one pretends otherwise.
		return "", doctorErrorf("could not preview fix for %s: secure randomness unavailable", checkID)
	}
	token := hex.EncodeToString(raw[:])
	doctorPreviewTokens.Lock()
	defer doctorPreviewTokens.Unlock()
	now := time.Now()
	for id, tok := range doctorPreviewTokens.byID {
		if now.Sub(tok.createdAt) > doctorPreviewTokenTTL {
			delete(doctorPreviewTokens.byID, id)
		}
	}
	doctorPreviewTokens.byID[token] = doctorPreviewToken{checkID: checkID, planHash: planHash, createdAt: now}
	return token, nil
}

// doctorPlanHash binds a preview token to the exact dry-run content it was
// minted for: summary plus every planned action, hashed.
func doctorPlanHash(summary string, actions []string) string {
	sum := sha256.Sum256([]byte(summary + "\x00" + strings.Join(actions, "\x00")))
	return hex.EncodeToString(sum[:])
}

// takeDoctorPreviewToken consumes the single-use token and returns the plan
// hash it was minted for. It succeeds only for the check the token was
// minted for and only within the TTL, so a token can neither authorize
// another check nor replay a stale plan. Consumption happens on every
// validated take (match or mismatch): a drifted plan must be re-previewed,
// never retried on the old token.
func takeDoctorPreviewToken(checkID string, token string) (string, error) {
	doctorPreviewTokens.Lock()
	defer doctorPreviewTokens.Unlock()
	stored, ok := doctorPreviewTokens.byID[token]
	if !ok {
		return "", doctorErrorf("risky fix for %s needs a fresh preview: call PreviewDoctorFix first", checkID)
	}
	if time.Since(stored.createdAt) > doctorPreviewTokenTTL {
		delete(doctorPreviewTokens.byID, token)
		return "", doctorErrorf("preview for %s expired: call PreviewDoctorFix again", checkID)
	}
	if stored.checkID != checkID {
		return "", doctorErrorf("preview token was minted for %s, not %s", stored.checkID, checkID)
	}
	delete(doctorPreviewTokens.byID, token)
	return stored.planHash, nil
}

// doctorFixKindForID classifies a check ID. known is false for IDs the
// allowlist has never seen, which callers reject fail-closed.
func doctorFixKindForID(checkID string) (kind DoctorFixKind, known bool) {
	if doctorSafeFixes[checkID] {
		return DoctorFixKindSafe, true
	}
	if doctorRiskyFixes[checkID] {
		return DoctorFixKindRisky, true
	}
	if doctorKnownNoFix[checkID] {
		return DoctorFixKindNone, true
	}
	return DoctorFixKindNone, false
}

func doctorErrorf(format string, args ...interface{}) error {
	return errors.New(sanitizeUIText(fmt.Sprintf(format, args...), doctorUIMessageMax))
}

func sanitizeDoctorTitle(s string) string {
	return sanitizeUIText(s, doctorUITitleMax)
}

func sanitizeDoctorMessage(s string) string {
	return sanitizeUIText(s, doctorUIMessageMax)
}

func sanitizeDoctorActions(actions []string) []string {
	cleaned := make([]string, 0, len(actions))
	for _, action := range actions {
		trimmed := strings.TrimSpace(sanitizeUIText(action, doctorUIActionMax))
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// RunDoctor runs the wrapped doctor engine (diagnostics only, never fixes)
// and classifies every check into the fix allowlist. The run registers an
// op so a long pass (hung docker probe) shows up in the op list and can be
// cancelled; diagnostics themselves are read-only, so a cancelled run
// leaves nothing behind.
func RunDoctor() (DoctorReportView, error) {
	opID := nextDoctorOpID("doctor")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterOp(opID, "", "doctor", cancel)
	defer FinishOp(opID)

	done := make(chan engine.DoctorReport, 1)
	// Capture the seam before spawning: a cancelled run's diagnostics
	// goroutine outlives the test that started it, and re-reading the
	// package var after the test's cleanup restored it is a data race.
	runDiagnostics := doctorRunDiagnosticsForDesktop
	go func() { done <- runDiagnostics() }()

	var raw engine.DoctorReport
	select {
	case raw = <-done:
	case <-ctx.Done():
		return DoctorReportView{}, doctorErrorf("doctor run was cancelled")
	}

	return buildDoctorReportView(raw, opID), nil
}

// RunDoctorForProject is RunDoctor scoped to a project root: the engine's
// project-bound checks read from that root instead of the process working
// directory, so the report the UI buckets under the selected project
// actually describes that project. An empty root keeps the legacy
// working-directory behavior.
func RunDoctorForProject(projectRoot string) (DoctorReportView, error) {
	opID := nextDoctorOpID("doctor")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterOp(opID, strings.TrimSpace(projectRoot), "doctor", cancel)
	defer FinishOp(opID)

	done := make(chan engine.DoctorReport, 1)
	// Same capture rule as RunDoctor: re-reading the package var after a
	// test's cleanup restored it is a data race.
	runDiagnostics := doctorRunDiagnosticsForProjectForDesktop
	root := strings.TrimSpace(projectRoot)
	go func() { done <- runDiagnostics(root) }()

	var raw engine.DoctorReport
	select {
	case raw = <-done:
	case <-ctx.Done():
		return DoctorReportView{}, doctorErrorf("doctor run was cancelled")
	}

	return buildDoctorReportView(raw, opID), nil
}

// buildDoctorReportView classifies every engine check into the fix
// allowlist and counts the outcomes. Shared by the unscoped and
// project-scoped runs so both surfaces classify identically.
func buildDoctorReportView(raw engine.DoctorReport, opID string) DoctorReportView {
	// OpID is the finished registry op: it lets the UI correlate the run,
	// and a late cancel of it is a harmless no-op by registry contract.
	report := DoctorReportView{Checks: make([]DoctorCheckView, 0, len(raw.Checks)), OpID: opID}
	for _, check := range raw.Checks {
		id := strings.TrimSpace(check.ID)
		kind, _ := doctorFixKindForID(id)
		view := DoctorCheckView{
			ID:               sanitizeDoctorTitle(id),
			Title:            sanitizeDoctorTitle(check.Title),
			Status:           sanitizeDoctorTitle(string(check.Status)),
			Message:          sanitizeDoctorMessage(check.Message),
			Hint:             sanitizeDoctorMessage(check.Hint),
			SuggestedCommand: sanitizeDoctorMessage(check.SuggestedCommand),
			FixKind:          kind,
		}
		switch kind {
		case DoctorFixKindSafe:
			view.FixLabel = "Fix"
		case DoctorFixKindRisky:
			view.FixLabel = "Preview fix"
			view.NeedsPreview = true
		}
		report.Checks = append(report.Checks, view)
		switch check.Status {
		case engine.DoctorStatusPass:
			report.Passed++
		case engine.DoctorStatusFail:
			report.Failures++
		default:
			report.Warnings++
		}
	}
	return report
}

// PreviewDoctorFix returns the dry-run plan plus a single-use preview token
// for a risky check. The preview never mutates: the executor runs in
// dry-run mode. Safe checks are refused (they apply directly) and none or
// unknown checks are refused (no fix path).
func PreviewDoctorFix(checkID string) (DoctorFixPreview, error) {
	return previewDoctorFix(checkID, "")
}

// doctorFixNeedsProjectScope reports whether a fix reads or writes project
// files: every risky fix, plus any safe fix outside the host-global
// allowlist (future-proofing: today every safe fix is host-global).
// Unknown and none checks return false so their fail-closed refusals keep
// their own wording.
func doctorFixNeedsProjectScope(checkID string) bool {
	id := strings.TrimSpace(checkID)
	kind, known := doctorFixKindForID(id)
	if !known || kind == DoctorFixKindNone {
		return false
	}
	if kind == DoctorFixKindRisky {
		return true
	}
	return !doctorSafeFixes[id]
}

// requireDoctorFixProjectScope hard-refuses a project-touching fix with an
// empty scope: without a selected project the executor would silently
// operate on the app's working directory. Host-global fixes may run
// explicitly unscoped. The unscoped legacy wrappers (PreviewDoctorFix,
// ApplyDoctorFix) keep their working-directory behavior for the hermetic
// Task 1 seam and CLI-like callers; every desktop-UI path goes through the
// ForProject variants or the HealthService, which both enforce this.
func requireDoctorFixProjectScope(checkID string, projectRoot string) error {
	if strings.TrimSpace(projectRoot) != "" || !doctorFixNeedsProjectScope(checkID) {
		return nil
	}
	return doctorErrorf("doctor fix for %s needs a project: select a project in the desktop app first (host-global fixes may run without one)", strings.TrimSpace(checkID))
}

// PreviewDoctorFixForProject is PreviewDoctorFix scoped to a project root:
// project-touching fixes read and plan against that root instead of the
// process working directory. An empty root is refused for project-touching
// fixes (see requireDoctorFixProjectScope); host-global fixes ignore it.
func PreviewDoctorFixForProject(checkID string, projectRoot string) (DoctorFixPreview, error) {
	if err := requireDoctorFixProjectScope(checkID, projectRoot); err != nil {
		return DoctorFixPreview{}, err
	}
	return previewDoctorFix(checkID, projectRoot)
}

func previewDoctorFix(checkID string, projectRoot string) (DoctorFixPreview, error) {
	id := strings.TrimSpace(checkID)
	kind, known := doctorFixKindForID(id)
	if !known {
		return DoctorFixPreview{}, doctorErrorf("unknown doctor check %q", id)
	}
	switch kind {
	case DoctorFixKindSafe:
		return DoctorFixPreview{}, doctorErrorf("safe fix for %s applies directly: call ApplyDoctorFix without a preview", id)
	case DoctorFixKindNone:
		return DoctorFixPreview{}, doctorErrorf("check %s has no fix available", id)
	}

	actions, summary, err := runDoctorFixExecutor(context.Background(), id, projectRoot, true)
	if err != nil {
		return DoctorFixPreview{}, doctorErrorf("preview for %s failed: %v", id, err)
	}
	token, err := mintDoctorPreviewToken(id, doctorPlanHash(summary, actions))
	if err != nil {
		return DoctorFixPreview{}, err
	}
	return DoctorFixPreview{
		PreviewID:       token,
		CheckID:         sanitizeDoctorTitle(id),
		Summary:         sanitizeDoctorMessage(summary),
		Actions:         sanitizeDoctorActions(actions),
		RequiresConfirm: true,
	}, nil
}

// ApplyDoctorFix applies one check's fix and reports the per-check outcome.
// Safe checks apply 1-click (previewID and confirm are ignored). Risky
// checks require the single-use token PreviewDoctorFix minted for that
// check plus confirm=true. None and unknown checks are refused before
// anything runs. Long applies register an op cancellable via
// CancelDoctorRun.
func ApplyDoctorFix(checkID string, previewID string, confirm bool) (DoctorFixOutcome, error) {
	return applyDoctorFix(checkID, previewID, confirm, "")
}

// ApplyDoctorFixForProject is ApplyDoctorFix scoped to a project root (see
// PreviewDoctorFixForProject).
func ApplyDoctorFixForProject(checkID string, previewID string, confirm bool, projectRoot string) (DoctorFixOutcome, error) {
	if err := requireDoctorFixProjectScope(checkID, projectRoot); err != nil {
		return DoctorFixOutcome{}, err
	}
	return applyDoctorFix(checkID, previewID, confirm, projectRoot)
}

func applyDoctorFix(checkID string, previewID string, confirm bool, projectRoot string) (DoctorFixOutcome, error) {
	id := strings.TrimSpace(checkID)
	kind, known := doctorFixKindForID(id)
	if !known {
		return DoctorFixOutcome{}, doctorErrorf("unknown doctor check %q", id)
	}
	var planHash string
	switch kind {
	case DoctorFixKindNone:
		return DoctorFixOutcome{}, doctorErrorf("check %s has no fix available", id)
	case DoctorFixKindRisky:
		if !confirm {
			return DoctorFixOutcome{}, doctorErrorf("risky fix for %s requires confirm: preview the fix and confirm explicitly", id)
		}
		var err error
		planHash, err = takeDoctorPreviewToken(id, strings.TrimSpace(previewID))
		if err != nil {
			return DoctorFixOutcome{}, err
		}
	}

	opID := nextDoctorOpID("doctor-fix")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterOp(opID, "", "doctor-fix", cancel)
	defer FinishOp(opID)

	if kind == DoctorFixKindRisky {
		if err := reverifyDoctorPreview(ctx, id, planHash, projectRoot); err != nil {
			return DoctorFixOutcome{}, err
		}
	}

	actions, message, err := runDoctorFixExecutor(ctx, id, projectRoot, false)
	if err != nil {
		if ctx.Err() != nil {
			return DoctorFixOutcome{}, doctorErrorf("fix for %s was cancelled", id)
		}
		return DoctorFixOutcome{}, doctorErrorf("fix for %s failed: %v", id, err)
	}
	return DoctorFixOutcome{
		CheckID: sanitizeDoctorTitle(id),
		Status:  DoctorFixOutcomeApplied,
		Message: sanitizeDoctorMessage(message),
		Actions: sanitizeDoctorActions(actions),
	}, nil
}

// CancelDoctorRun cancels a still-running doctor run or fix via the op
// registry. Cancelling a finished or unknown id is a harmless no-op.
func CancelDoctorRun(opID string) bool {
	return CancelOp(strings.TrimSpace(opID))
}

// reverifyDoctorPreview recomputes the dry-run plan at apply time and
// compares its hash with the one the preview token was minted for. State
// that drifted in between is refused with a re-preview hint before
// anything mutates. The token is already consumed (take), so a refused
// apply needs a fresh preview either way.
func reverifyDoctorPreview(ctx context.Context, checkID string, planHash string, projectRoot string) error {
	actions, summary, err := runDoctorFixExecutor(ctx, checkID, projectRoot, true)
	if err != nil {
		if ctx.Err() != nil {
			return doctorErrorf("fix for %s was cancelled", checkID)
		}
		return doctorErrorf("fix for %s failed: could not re-verify preview: %v", checkID, err)
	}
	if doctorPlanHash(summary, actions) != planHash {
		return doctorErrorf("state changed since preview for %s: please re-preview", checkID)
	}
	return nil
}

// runDoctorFixExecutor runs (dryRun=false) or plans (dryRun=true) one fix.
// An empty projectRoot keeps the legacy process-working-directory behavior
// and honors the test seam; a non-empty root scopes the real dispatcher to
// that project directory instead. Scoped runs always use the real
// dispatcher: the stub seam exists for hermetic allowlist/token tests, which
// run unscoped, while root scoping is a property of the default executor
// covered against real temp project directories.
func runDoctorFixExecutor(ctx context.Context, checkID string, projectRoot string, dryRun bool) ([]string, string, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return doctorFixExecutorForDesktop(ctx, checkID, dryRun)
	}
	return dispatchDoctorFixInRoot(ctx, checkID, dryRun, root)
}

// doctorWorkDir resolves the directory project-touching fixes operate in:
// the scoped root, or the process working directory for legacy unscoped
// runs (exactly like `govard doctor --fix` from a project root).
func doctorWorkDir(projectRoot string) string {
	if strings.TrimSpace(projectRoot) == "" {
		wd, _ := os.Getwd()
		return wd
	}
	return projectRoot
}

// doctorConfigPath resolves the project config file project-touching fixes
// read and write: inside the scoped root, or the legacy working-directory
// relative path for unscoped runs.
func doctorConfigPath(projectRoot string) string {
	if strings.TrimSpace(projectRoot) == "" {
		return conventions.BaseConfigFile
	}
	return filepath.Join(projectRoot, conventions.BaseConfigFile)
}

// dispatchDoctorFix routes one allowlisted check to its executor. Unknown
// IDs are unreachable through Preview/ApplyDoctorFix (fail-closed above),
// so the default here is defensive.
func dispatchDoctorFix(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
	return dispatchDoctorFixInRoot(ctx, checkID, dryRun, "")
}

// dispatchDoctorFixInRoot is dispatchDoctorFix scoped to a project root:
// host-global fixes ignore the root, project-touching fixes resolve their
// files from it instead of the process working directory.
func dispatchDoctorFixInRoot(ctx context.Context, checkID string, dryRun bool, projectRoot string) ([]string, string, error) {
	switch checkID {
	case "host.govard.home":
		return fixDoctorGovardHome(ctx, dryRun)
	case "host.compose.spam":
		return fixDoctorComposeSpam(ctx, dryRun)
	case "host.govard.registry":
		return fixDoctorRegistry(ctx, dryRun)
	case "host.search.index_block":
		return fixDoctorSearchIndex(ctx, dryRun, projectRoot)
	case "project.runtime.images":
		return fixDoctorRuntimeImages(ctx, dryRun, projectRoot)
	case "project.config.audit":
		return fixDoctorConfigAudit(ctx, dryRun, projectRoot)
	case "project.config.drift":
		return fixDoctorConfigDrift(ctx, dryRun, projectRoot)
	case "project.profile.sync":
		return fixDoctorProfileSync(ctx, dryRun, projectRoot)
	default:
		return nil, "", fmt.Errorf("no fix executor for check %q", checkID)
	}
}

func doctorFixCancelled(ctx context.Context) error {
	return ctx.Err()
}

// fixDoctorGovardHome repairs Govard runtime directories (safe: permission
// repair). Mirrors the CLI fix without reimplementing its diagnostics.
func fixDoctorGovardHome(ctx context.Context, dryRun bool) ([]string, string, error) {
	homeDir := filepath.Clean(engine.GovardHomeDir())
	dirs := []string{homeDir, filepath.Join(homeDir, "compose"), filepath.Join(homeDir, "diagnostics")}
	actions := []string{}
	for _, dir := range dirs {
		actions = append(actions, fmt.Sprintf("mkdir -p %s", dir), fmt.Sprintf("chmod 700 %s", dir))
	}
	actions = append(actions, fmt.Sprintf("write probe file in %s", homeDir))
	if dryRun {
		return actions, "[dry-run] would repair Govard home directories", nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, conventions.SecretDirPerm); err != nil {
			return nil, "", err
		}
		if err := os.Chmod(dir, conventions.SecretDirPerm); err != nil {
			return nil, "", err
		}
	}
	if err := engine.CheckGovardHomeWritable(); err != nil {
		return nil, "", err
	}
	return actions, "Govard runtime directories prepared.", nil
}

// fixDoctorComposeSpam prunes stale compose files (safe: prune).
func fixDoctorComposeSpam(ctx context.Context, dryRun bool) ([]string, string, error) {
	actions := []string{"Purging compose files older than 7 days"}
	if dryRun {
		return actions, "[dry-run] would purge stale compose files", nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	count, err := engine.CleanupStaleComposeFiles(7 * 24 * time.Hour)
	if err != nil {
		return nil, "", err
	}
	return actions, fmt.Sprintf("Removed %d stale compose file(s).", count), nil
}

// fixDoctorRegistry repairs a corrupt project registry path (safe:
// permission/path repair): an absent or healthy file needs no action. A
// directory at the file path is refused, never auto-deleted: that shape
// means corruption for human eyes, and rm -rf is not a 1-click operation.
func fixDoctorRegistry(ctx context.Context, dryRun bool) ([]string, string, error) {
	path := engine.ProjectRegistryPath()
	if dryRun {
		return []string{fmt.Sprintf("Repair registry path %s", path)}, "[dry-run] would repair the project registry path", nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"Registry path absent: nothing to repair"}, "Project registry is healthy.", nil
		}
		return nil, "", err
	}
	if !info.IsDir() {
		return []string{"Registry file present: nothing to repair"}, "Project registry is healthy.", nil
	}
	return nil, "", fmt.Errorf("registry path %s is a directory: manual repair required (nothing was deleted)", path)
}

// fixDoctorSearchIndex unblocks the search index (risky: it issues a write
// into the running search container, so it is never 1-click). Non-
// interactive: the preview+confirm gate replaces the CLI's prompts.
func fixDoctorSearchIndex(ctx context.Context, dryRun bool, projectRoot string) ([]string, string, error) {
	actions := []string{"unblock search index via docker exec curl"}
	if dryRun {
		return actions, "[dry-run] would unblock the search index", nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	wd := doctorWorkDir(projectRoot)
	config, err := engine.LoadRawConfigFromDir(wd, false)
	if err != nil {
		return nil, "", err
	}
	definition, ok := frameworks.Get(config.Framework)
	if !ok || definition.UnblockSearchIndex == nil {
		return nil, "", fmt.Errorf("%s does not support this search-index fix", config.Framework)
	}
	if err := definition.UnblockSearchIndex(config); err != nil {
		return nil, "", err
	}
	return actions, "Search index unblocked.", nil
}

// fixDoctorRuntimeImages pulls missing runtime images (risky: network and
// volume-heavy). The dry run inspects (read-only) and plans; the apply
// pulls with ctx cancellation.
func fixDoctorRuntimeImages(ctx context.Context, dryRun bool, projectRoot string) ([]string, string, error) {
	wd := doctorWorkDir(projectRoot)
	config, _, err := engine.LoadConfigFromDir(wd, true)
	if err != nil {
		if strings.Contains(err.Error(), conventions.BaseConfigFile+" not found") {
			return []string{"No project configuration: no images required"}, "No project configuration found.", nil
		}
		return nil, "", err
	}
	required := engine.RequiredRuntimeImages(config, wd)
	missing := []string{}
	for _, image := range required {
		if err := exec.CommandContext(ctx, "docker", "image", "inspect", image).Run(); err != nil {
			missing = append(missing, image)
		}
	}
	if len(missing) == 0 {
		return []string{"All required images are already present"}, "All required Docker runtime images are cached locally.", nil
	}
	actions := make([]string, 0, len(missing))
	for _, image := range missing {
		actions = append(actions, fmt.Sprintf("Pull %s", image))
	}
	if dryRun {
		return actions, fmt.Sprintf("[dry-run] would pull %d missing image(s)", len(missing)), nil
	}
	for _, image := range missing {
		cmd := exec.CommandContext(ctx, "docker", "pull", image)
		if out, err := cmd.CombinedOutput(); err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", fmt.Errorf("failed to pull image %s: %s", image, strings.TrimSpace(string(out)))
		}
	}
	return actions, fmt.Sprintf("Pulled %d missing image(s).", len(missing)), nil
}

// doctorLegacyKeys detects legacy .govard.yml keys without mutating.
func doctorLegacyKeys(data []byte) []string {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	stackRaw, ok := raw["stack"].(map[string]interface{})
	if !ok {
		return nil
	}
	found := []string{}
	if _, ok := stackRaw["db"]; ok {
		found = append(found, "stack.db")
	}
	if featuresRaw, ok := stackRaw["features"].(map[string]interface{}); ok {
		for _, key := range []string{"redis", "elasticsearch", "rabbitmq"} {
			if _, ok := featuresRaw[key]; ok {
				found = append(found, "features."+key)
			}
		}
	}
	return found
}

// fixDoctorConfigAudit migrates legacy config keys (risky: config write).
func fixDoctorConfigAudit(ctx context.Context, dryRun bool, projectRoot string) ([]string, string, error) {
	configPath := doctorConfigPath(projectRoot)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, "", err
	}
	legacy := doctorLegacyKeys(data)
	if len(legacy) == 0 {
		return []string{"No legacy keys: nothing to migrate"}, "Configuration file is using current naming standards.", nil
	}
	actions := make([]string, 0, len(legacy))
	for _, key := range legacy {
		actions = append(actions, fmt.Sprintf("Migrate legacy key '%s' to the services block", key))
	}
	if dryRun {
		return actions, "[dry-run] would migrate legacy config", nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, "", fmt.Errorf("failed to parse yaml: %w", err)
	}
	stackRaw, ok := raw["stack"].(map[string]interface{})
	if !ok {
		return nil, "", fmt.Errorf("no stack configuration found to migrate")
	}
	servicesRaw, ok := stackRaw["services"].(map[string]interface{})
	if !ok {
		servicesRaw = make(map[string]interface{})
		stackRaw["services"] = servicesRaw
	}
	if dbType, ok := stackRaw["db"]; ok {
		if servicesRaw["db"] == nil || servicesRaw["db"] == "" {
			servicesRaw["db"] = dbType
		}
		delete(stackRaw, "db")
	}
	if featuresRaw, ok := stackRaw["features"].(map[string]interface{}); ok {
		migrations := map[string]string{"redis": "cache", "elasticsearch": "search", "rabbitmq": "queue"}
		defaults := map[string]string{"redis": "redis", "elasticsearch": "elasticsearch", "rabbitmq": "rabbitmq"}
		for legacyKey, service := range migrations {
			if val, exists := featuresRaw[legacyKey]; exists {
				if b, ok := val.(bool); ok && b {
					if servicesRaw[service] == nil || servicesRaw[service] == "" {
						servicesRaw[service] = defaults[legacyKey]
					}
				}
				delete(featuresRaw, legacyKey)
			}
		}
	}
	var cfg engine.Config
	tempData, _ := yaml.Marshal(&raw)
	_ = yaml.Unmarshal(tempData, &cfg)
	writableConfig := engine.PrepareConfigForWrite(cfg)
	updated, err := yaml.Marshal(&writableConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal updated config: %w", err)
	}
	if err := os.WriteFile(configPath, updated, conventions.DefaultFilePerm); err != nil {
		return nil, "", fmt.Errorf("failed to write updated config: %w", err)
	}
	return actions, "Legacy configuration migrated to new standard.", nil
}

// fixDoctorConfigDrift syncs .govard.yml with the detected profile (risky:
// config write). The dry run uses the engine's own no-write planning.
func fixDoctorConfigDrift(ctx context.Context, dryRun bool, projectRoot string) ([]string, string, error) {
	wd := doctorWorkDir(projectRoot)
	changes, err := engine.SyncConfigDriftForTest(wd, true, false)
	if err != nil {
		return nil, "", err
	}
	if len(changes) == 0 {
		return []string{"No drift detected"}, "Configuration is in sync with framework profile.", nil
	}
	actions := make([]string, 0, len(changes))
	for _, change := range changes {
		actions = append(actions, fmt.Sprintf("yq update .govard.yml %s", change))
	}
	if dryRun {
		return actions, fmt.Sprintf("[dry-run] would sync: %s", strings.Join(changes, ", ")), nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	if _, err := engine.SyncConfigDriftForTest(wd, false, false); err != nil {
		return nil, "", err
	}
	return actions, fmt.Sprintf("Synced %s", strings.Join(changes, ", ")), nil
}

// fixDoctorProfileSync tunes the environment to the framework profile
// (risky: config write). Non-interactive subset of the CLI tune: explicit
// user settings are always preserved (the CLI's override-reset prompts
// stay a manual/CLI flow), gaps are filled from the profile.
func fixDoctorProfileSync(ctx context.Context, dryRun bool, projectRoot string) ([]string, string, error) {
	wd := doctorWorkDir(projectRoot)
	config, err := engine.LoadRawConfigFromDir(wd, false)
	if err != nil {
		return nil, "", err
	}
	if config.Framework == "" || config.Framework == "generic" || config.Framework == "custom" {
		return []string{"No framework profile applies"}, "No framework profile to sync.", nil
	}
	metadata := engine.DetectFramework(wd)
	warnings := engine.CollectProfileSyncWarnings(config, metadata)
	if len(warnings) == 0 {
		return []string{"Profile already in sync"}, "Runtime environment is synced with the recommended framework profile.", nil
	}
	actions := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		actions = append(actions, fmt.Sprintf("Sync %s", warning))
	}
	if dryRun {
		return actions, fmt.Sprintf("[dry-run] would sync %d profile setting(s), preserving explicit user settings", len(warnings)), nil
	}
	if err := doctorFixCancelled(ctx); err != nil {
		return nil, "", err
	}
	originalBytes, _ := os.ReadFile(doctorConfigPath(projectRoot))
	version := strings.TrimSpace(metadata.Version)
	if version == "" {
		version = strings.TrimSpace(config.FrameworkVersion)
	}
	profileResult, err := engine.ResolveRuntimeProfile(config.Framework, version)
	if err != nil {
		return nil, "", err
	}
	existing := config
	engine.ApplyRuntimeProfileToConfig(&config, profileResult.Profile)
	// Preserve every explicit user setting: only profile gaps are filled.
	if existing.Stack.PHPVersion != "" {
		config.Stack.PHPVersion = existing.Stack.PHPVersion
	} else {
		config.Stack.PHPVersion = ""
	}
	if existing.Stack.NodeVersion != "" {
		config.Stack.NodeVersion = existing.Stack.NodeVersion
	} else {
		config.Stack.NodeVersion = ""
	}
	if existing.Stack.ComposerVersion != "" {
		config.Stack.ComposerVersion = existing.Stack.ComposerVersion
	}
	if existing.Stack.DBVersion != "" {
		config.Stack.DBVersion = existing.Stack.DBVersion
	} else {
		config.Stack.DBVersion = ""
	}
	engine.NormalizeConfig(&config, wd)
	if existing.Stack.Services.Cache == "none" || existing.Stack.Services.Cache == "" {
		config.Stack.Services.Cache = "none"
		config.Stack.CacheVersion = ""
		config.Stack.Features.Cache = false
	} else {
		config.Stack.Services.Cache = existing.Stack.Services.Cache
		config.Stack.CacheVersion = existing.Stack.CacheVersion
	}
	if existing.Stack.Services.Search == "none" || existing.Stack.Services.Search == "" {
		config.Stack.Services.Search = "none"
		config.Stack.SearchVersion = ""
		config.Stack.Features.Search = false
	} else {
		config.Stack.Services.Search = existing.Stack.Services.Search
		config.Stack.SearchVersion = existing.Stack.SearchVersion
	}
	if existing.Stack.Services.Queue == "none" || existing.Stack.Services.Queue == "" {
		config.Stack.Services.Queue = "none"
		config.Stack.QueueVersion = ""
		config.Stack.Features.Queue = false
	} else {
		config.Stack.Services.Queue = existing.Stack.Services.Queue
		config.Stack.QueueVersion = existing.Stack.QueueVersion
	}
	writableConfig := engine.PrepareConfigForWrite(config)
	updated, err := yaml.Marshal(&writableConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal config: %w", err)
	}
	if string(updated) == string(originalBytes) {
		return actions, "Advisory: explicit user settings preserved (no profile changes to apply).", nil
	}
	if err := os.WriteFile(doctorConfigPath(projectRoot), updated, conventions.DefaultFilePerm); err != nil {
		return nil, "", fmt.Errorf("failed to write updated config: %w", err)
	}
	return actions, fmt.Sprintf("Environment updated to %s profile (%s)", config.Framework, profileResult.Source), nil
}
