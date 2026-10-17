package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"govard/internal/audit"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
	"govard/internal/projectstores"
)

// AuditService backend (Task 2, WS-E health UX): wraps the existing audit
// integrity paths so the desktop health header can run them docker-free.
// It builds an audit.Runner with an integrity-only request (the same
// container-free analyzers `govard audit run --checks integrity` executes),
// never a lint or profiler backend, and reimplements nothing.
//
// Docker-backed checks are never run from the desktop and never hidden:
// every result carries lint and profiler rows with Enabled=false and a
// DisabledReason pointing at the CLI. Long runs register an op (kind
// "audit") with audit:progress events and real cancel through CancelAudit.
// The last completed result per project is indexed in memory for banner use
// by Task 3 (the audit store on disk stays the durable record).
//
// This file stays binding-free (plain package functions, no Wails service
// methods) so the bindings contract tests are unaffected until Task 3 binds
// these to the frontend.

// AuditCheckView is one audit check row for the UI. Docker-backed checks
// render with Enabled=false and a DisabledReason, never hidden.
type AuditCheckView struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabledReason,omitempty"`
}

// AuditFindingView is one sanitized integrity finding for the UI.
type AuditFindingView struct {
	Tool    string `json:"tool"`
	Rule    string `json:"rule,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// AuditResultView is the sanitized integrity run result the health header
// renders and the banner reads back via LastAuditResult. ProjectRoot is the
// normalized (absolute, symlink-resolved, cleaned) project root the run was
// recorded under; OpID is the historical registry op of the finished run (a
// late CancelAudit for it is a harmless no-op).
type AuditResultView struct {
	ProjectID    string             `json:"projectId"`
	ProjectRoot  string             `json:"projectRoot,omitempty"`
	SessionID    string             `json:"sessionId"`
	RunID        string             `json:"runId"`
	Status       string             `json:"status"`
	FindingCount int                `json:"findingCount"`
	FinishedAt   time.Time          `json:"finishedAt"`
	Checks       []AuditCheckView   `json:"checks"`
	Findings     []AuditFindingView `json:"findings"`
	OpID         string             `json:"opId"`
}

// AuditProgressPayload is the structured audit:progress event payload.
// ProjectRoot is the normalized project root (normalizeAuditProjectRoot,
// the same identity the banner index keys on) the run was launched for, so
// the UI can drop events from other scopes instead of showing project A's
// progress on project B's panel. It is always set on desktop emits; an
// empty value means a legacy or foreign sender the UI must handle by its
// documented unscoped rule.
type AuditProgressPayload struct {
	OpID        string `json:"opId"`
	Phase       string `json:"phase"`
	Percent     int    `json:"percent"`
	Message     string `json:"message"`
	SessionID   string `json:"sessionId,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

// Audit progress phases emitted on audit:progress.
const (
	auditPhaseRunning   = "running"
	auditPhaseDone      = "done"
	auditPhaseFailed    = "failed"
	auditPhaseCancelled = "cancelled"
)

// auditDockerDisabledReason is the shared explanation on every docker-backed
// check row: present but disabled, never hidden, per the plan's global
// constraints.
const auditDockerDisabledReason = "Requires Docker and is not run from the desktop"

// UI text caps: finding text comes from project files and analyzer output
// and can be arbitrarily long; the views stay toast-sized.
const (
	auditUIIDMax      = 120
	auditUIPathMax    = 280
	auditUIMessageMax = 500
	auditUIStatusMax  = 40
)

// resolveAuditRunRequestForDesktop resolves the docker-free audit context
// for a project root (empty means the process working directory, exactly
// like `govard audit run` from a project root). It mirrors the CLI's target
// resolution minus everything container-backed: no lint provider selection,
// no PHP policy probing, no profiler runtime. The caller sets Checks
// (always exactly [integrity]); the seam returns the context without them.
var resolveAuditRunRequestForDesktop = defaultResolveAuditRunRequest

// runAuditRunnerForDesktop executes the resolved request against the audit
// runner. Production wraps audit.Runner.Run with the integrity-only request,
// which builds just the host-analysis job (no lint backend, no profiler
// runtime, no container).
var runAuditRunnerForDesktop = defaultRunAuditRunner

// readAuditRunResultForDesktop reads a persisted run. An empty runID
// resolves to the session's latest run. Production reads the audit store.
var readAuditRunResultForDesktop = defaultReadAuditRunResult

// listAuditRunIDsForDesktop lists a session's run IDs in oldest-first order.
// Production reads the session manifest.
var listAuditRunIDsForDesktop = defaultListAuditRunIDs

// auditPreRecordHookForDesktop is a test-only seam (nil in production) that
// runs synchronously between the completion check and the banner record, so
// tests can deliver CancelAudit deterministically inside that race window
// without sleeps. Production has no hook: the shared-lock re-check in
// recordLastAuditResultUnlessCancelled is the real guarantee.
var auditPreRecordHookForDesktop func(opID string)

// defaultListAuditRunIDs lists a session's run IDs oldest-first from the
// audit store.
func defaultListAuditRunIDs(projectID, sessionID string) ([]string, error) {
	store := audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir()))
	manifest, err := store.ReadSession(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(manifest.Runs))
	for _, run := range manifest.Runs {
		if strings.TrimSpace(run.RunID) != "" {
			ids = append(ids, run.RunID)
		}
	}
	return ids, nil
}

// AuditDiffView is the previous-vs-current integrity finding summary over two
// stored runs. Findings are matched by tool, rule, path, line, and message:
// a finding that moved lines reads as one removal plus one addition, which is
// honest for a summary (the code did change under it).
type AuditDiffView struct {
	ProjectID      string             `json:"projectId"`
	SessionID      string             `json:"sessionId"`
	FromRunID      string             `json:"fromRunId"`
	ToRunID        string             `json:"toRunId"`
	FromStatus     string             `json:"fromStatus"`
	ToStatus       string             `json:"toStatus"`
	Added          []AuditFindingView `json:"added"`
	Removed        []AuditFindingView `json:"removed"`
	UnchangedCount int                `json:"unchangedCount"`
}

// GetAuditDiff summarizes the integrity finding delta between two stored runs
// of a session. An empty toRunID resolves to the latest run; an empty
// fromRunID resolves to the run before it. There is no engine result-diff
// entry point to wrap (the audit package diffs scopes at run time, not stored
// results), so this compares the two persisted integrity finding sets. A run
// diffed against itself is refused; a session with no earlier run has nothing
// to diff against.
func GetAuditDiff(projectID, sessionID, fromRunID, toRunID string) (AuditDiffView, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	fromRunID = strings.TrimSpace(fromRunID)
	toRunID = strings.TrimSpace(toRunID)
	if projectID == "" || sessionID == "" {
		return AuditDiffView{}, auditErrorf("audit diff requires a project and a session")
	}
	if toRunID == "" || fromRunID == "" {
		ids, err := listAuditRunIDsForDesktop(projectID, sessionID)
		if err != nil {
			return AuditDiffView{}, auditErrorf("audit diff: %v", err)
		}
		if len(ids) == 0 {
			return AuditDiffView{}, auditErrorf("audit diff: session %s has no runs", sessionID)
		}
		if toRunID == "" {
			toRunID = ids[len(ids)-1]
		}
		if fromRunID == "" {
			previous := ""
			for _, id := range ids {
				if id == toRunID {
					break
				}
				previous = id
			}
			if previous == "" {
				return AuditDiffView{}, auditErrorf("audit diff: session %s has no earlier run to diff against", sessionID)
			}
			fromRunID = previous
		}
	}
	if fromRunID == toRunID {
		return AuditDiffView{}, auditErrorf("audit diff: cannot diff run %s against itself", fromRunID)
	}
	fromResult, err := readAuditRunResultForDesktop(projectID, sessionID, fromRunID)
	if err != nil {
		return AuditDiffView{}, auditErrorf("audit diff: %v", err)
	}
	toResult, err := readAuditRunResultForDesktop(projectID, sessionID, toRunID)
	if err != nil {
		return AuditDiffView{}, auditErrorf("audit diff: %v", err)
	}
	fromFindings := auditFindingsFromResult(fromResult)
	toFindings := auditFindingsFromResult(toResult)
	previous := make(map[string]AuditFindingView, len(fromFindings))
	for _, finding := range fromFindings {
		previous[auditFindingKey(finding)] = finding
	}
	current := make(map[string]AuditFindingView, len(toFindings))
	for _, finding := range toFindings {
		current[auditFindingKey(finding)] = finding
	}
	diff := AuditDiffView{
		ProjectID:  sanitizeAuditID(projectID),
		SessionID:  sanitizeAuditID(sessionID),
		FromRunID:  sanitizeAuditID(fromRunID),
		ToRunID:    sanitizeAuditID(toRunID),
		FromStatus: sanitizeAuditStatus(string(fromResult.Status)),
		ToStatus:   sanitizeAuditStatus(string(toResult.Status)),
	}
	for key, finding := range current {
		if _, ok := previous[key]; ok {
			diff.UnchangedCount++
		} else {
			diff.Added = append(diff.Added, finding)
		}
	}
	for key, finding := range previous {
		if _, ok := current[key]; !ok {
			diff.Removed = append(diff.Removed, finding)
		}
	}
	// Map iteration order is nondeterministic: sort both slices by finding
	// key so multi-item diffs render in a stable order.
	sort.Slice(diff.Added, func(i, j int) bool {
		return auditFindingKey(diff.Added[i]) < auditFindingKey(diff.Added[j])
	})
	sort.Slice(diff.Removed, func(i, j int) bool {
		return auditFindingKey(diff.Removed[i]) < auditFindingKey(diff.Removed[j])
	})
	if diff.Added == nil {
		diff.Added = []AuditFindingView{}
	}
	if diff.Removed == nil {
		diff.Removed = []AuditFindingView{}
	}
	return diff, nil
}

// auditFindingsFromResult extracts the sanitized integrity findings of one
// stored or just-completed run, ignoring other jobs.
func auditFindingsFromResult(result audit.RunResult) []AuditFindingView {
	for _, job := range result.Jobs {
		if strings.TrimSpace(job.ID) != audit.IntegrityCheck {
			continue
		}
		return auditFindingsFromEvidence(job.Evidence)
	}
	return nil
}

// auditFindingKey identifies a finding across runs for summary diffing.
func auditFindingKey(finding AuditFindingView) string {
	return finding.Tool + "\x00" + finding.Rule + "\x00" + finding.Path + "\x00" + strconv.Itoa(finding.Line) + "\x00" + finding.Message
}

// auditOpSequence makes registry op ids unique across runs.
var auditOpSequence atomic.Uint64

func nextAuditOpID(prefix string) string {
	return prefix + ":" + strconv.FormatUint(auditOpSequence.Add(1), 10)
}

// All desktop audit state is keyed by resolved absolute project-root path
// (Abs plus EvalSymlinks plus Clean), never by display spelling: a trailing
// slash, a "/./" segment, or a relative form resolves to the same entry. An
// empty root means the process working directory, matching RunIntegrityAudit.
//
// NOTE(WS-D): the canonical project ID will supersede path keys once it is
// defined; until then path keys are the rule, with the runner's project hash
// kept inside each record for store correlation. A path-keyed lookup that
// misses never falls back to guessing another project's entry.
func normalizeAuditProjectRoot(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		workdir, err := os.Getwd()
		if err != nil {
			return ""
		}
		trimmed = workdir
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		absolute = trimmed
	} else if resolved, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute)
}

// auditLastResultsFileName is the persisted banner index: the last completed
// integrity result per normalized project root. The audit store on disk stays
// the durable run record; this file is only the banner's shortcut to it.
const auditLastResultsFileName = "audit-last-results.json"

func auditLastResultsPath() string {
	return filepath.Join(engine.GovardHomeDir(), "desktop", auditLastResultsFileName)
}

// auditCancelledMark is one cancellation request: whether a CancelOp for the
// op succeeded (won) and when the request was first marked (at), the latter
// so the opportunistic age-purge can drop marks no live path will ever
// consume. Won marks are never purged: a won mark with a still-running op
// must survive indefinitely (a cancelled long audit keeps running in the
// background by design), and a won mark is always consumed by its op's record
// or early-discard path.
type auditCancelledMark struct {
	won bool
	at  time.Time
}

// auditCancelledMarkMaxAge bounds the cancelled-set: inserts drop
// unacknowledged marks older than this. Only never-acknowledged marks are
// eligible — a mark unacknowledged for over an hour belongs to an op whose
// CancelOp settled as failed long ago (any later success re-arms via settle),
// so dropping it cannot promote a wrongly-cancelled run.
const auditCancelledMarkMaxAge = time.Hour

// lastAuditResultsCache is the in-memory copy of the persisted banner index,
// reloaded from disk whenever the Govard home changes (which covers process
// restart as well as per-test home isolation). It also owns the
// cancelled-set. Both the cancelling side (CancelAudit) and the recording
// side mutate the set under THIS mutex only — no registry lock is taken
// inside the banner lock anywhere — so a cancel delivered at any point before
// the record critical section is observed by it, and a record completed
// before the mark is a completed run the cancel simply missed. Entries are
// consumed by the record (success or discard), by the early-discard path, by
// failed cancels, and by the post-FinishOp scrub, so the set stays bounded by
// in-flight ops.
var lastAuditResultsCache = struct {
	sync.Mutex
	home      string
	loaded    bool
	byRoot    map[string]AuditResultView
	cancelled map[string]auditCancelledMark
}{byRoot: map[string]AuditResultView{}, cancelled: map[string]auditCancelledMark{}}

// resetLastAuditResults clears the in-memory copy only and keeps the disk
// file: that is exactly a service restart, which the persistence test uses.
func resetLastAuditResults() {
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	lastAuditResultsCache.home = ""
	lastAuditResultsCache.loaded = false
	lastAuditResultsCache.byRoot = map[string]AuditResultView{}
	lastAuditResultsCache.cancelled = map[string]auditCancelledMark{}
}

// ensureLastAuditResultsLoadedLocked reloads the disk copy when this is the
// first access or the Govard home moved since the last load. A missing or
// corrupt file means no banner state, never an error: the next completed run
// rewrites it.
func ensureLastAuditResultsLoadedLocked() {
	home := engine.GovardHomeDir()
	if lastAuditResultsCache.loaded && lastAuditResultsCache.home == home {
		return
	}
	lastAuditResultsCache.home = home
	lastAuditResultsCache.loaded = true
	lastAuditResultsCache.byRoot = map[string]AuditResultView{}
	data, err := os.ReadFile(auditLastResultsPath())
	if err != nil || len(data) == 0 {
		return
	}
	_ = json.Unmarshal(data, &lastAuditResultsCache.byRoot)
}

// saveLastAuditResultsLocked persists the banner index best-effort with an
// atomic write (temp file plus rename). A persistence failure must never fail
// the audit run it follows, so errors are dropped here; the run's durable
// record already lives in the audit store.
func saveLastAuditResultsLocked() {
	path := auditLastResultsPath()
	if err := os.MkdirAll(filepath.Dir(path), conventions.DefaultDirPerm); err != nil {
		return
	}
	data, err := json.MarshalIndent(lastAuditResultsCache.byRoot, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "audit-last-results-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, path)
}

// recordLastAuditResultUnlessCancelled records the completed result under
// the same banner-index lock the cancelling side marks: the cancelled-set is
// re-examined inside the record critical section, so a cancel delivered after
// RunIntegrityAudit's early completion check still discards the late result
// instead of promoting it. The set alone is sufficient — no context check
// here: a dead context implies a won mark is present (the context fires only
// on the CancelOp-true path, which always arms the set, and cleanup never
// removes a won mark), and a live context with a mark means the cancel is
// still in flight. A consumed mark is deleted, and a successful record
// deletes any mark too, so entries never outlive their op. It reports whether
// the result was recorded and whether the refusal (if any) was a cancellation
// as opposed to an unkeyable root.
func recordLastAuditResultUnlessCancelled(root string, view AuditResultView, opID string) (recorded bool, wasCancelled bool) {
	key := normalizeAuditProjectRoot(root)
	if key == "" {
		return false, false
	}
	view.ProjectRoot = key
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	ensureLastAuditResultsLoadedLocked()
	if _, ok := lastAuditResultsCache.cancelled[opID]; ok {
		delete(lastAuditResultsCache.cancelled, opID)
		return false, true
	}
	lastAuditResultsCache.byRoot[key] = view
	delete(lastAuditResultsCache.cancelled, opID)
	saveLastAuditResultsLocked()
	return true, false
}

// purgeStaleAuditCancelledLocked drops never-acknowledged marks older than
// auditCancelledMarkMaxAge. It runs on inserts only, under the banner lock.
// Won marks are never eligible (see auditCancelledMark): purging one could
// promote a run whose cancel is still legitimately in flight.
func purgeStaleAuditCancelledLocked(now time.Time) {
	for id, mark := range lastAuditResultsCache.cancelled {
		if !mark.won && now.Sub(mark.at) > auditCancelledMarkMaxAge {
			delete(lastAuditResultsCache.cancelled, id)
		}
	}
}

// markAuditOpCancelled records a cancellation request under the banner lock
// only (never the registry lock) and reports whether the entry is new.
// CancelAudit calls this BEFORE CancelOp, so every record critical section
// after this point observes the request.
func markAuditOpCancelled(opID string) bool {
	if opID == "" {
		return false
	}
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	if _, present := lastAuditResultsCache.cancelled[opID]; present {
		return false
	}
	now := time.Now()
	lastAuditResultsCache.cancelled[opID] = auditCancelledMark{at: now}
	purgeStaleAuditCancelledLocked(now)
	return true
}

// settleAuditOpCancelled reconciles the mark with the CancelOp outcome under
// the banner lock only: a success arms it (create or overwrite, so a
// concurrent canceller's cleanup cannot lose it — including the case where
// the record already consumed the entry, i.e. a cancel acknowledged strictly
// after record, which the post-FinishOp scrub then removes); a failure
// removes a mark this call added unless a concurrent success armed it
// meanwhile.
func settleAuditOpCancelled(opID string, cancelOK bool, newlyMarked bool) {
	if opID == "" {
		return
	}
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	if cancelOK {
		mark := lastAuditResultsCache.cancelled[opID]
		if mark.at.IsZero() {
			mark.at = time.Now()
		}
		mark.won = true
		lastAuditResultsCache.cancelled[opID] = mark
		purgeStaleAuditCancelledLocked(time.Now())
		return
	}
	if newlyMarked {
		if mark := lastAuditResultsCache.cancelled[opID]; !mark.won {
			delete(lastAuditResultsCache.cancelled, opID)
		}
	}
}

// takeAuditOpCancelled consumes a pending request under the banner lock,
// reporting whether one existed. The early completion-discard path calls it
// so a cancelled run leaves no residue for an op that will never record.
func takeAuditOpCancelled(opID string) bool {
	if opID == "" {
		return false
	}
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	if _, ok := lastAuditResultsCache.cancelled[opID]; !ok {
		return false
	}
	delete(lastAuditResultsCache.cancelled, opID)
	return true
}

// forgetAuditOpCancelled drops any mark for the op under the banner lock.
// RunIntegrityAudit defers it after FinishOp so a cancel acknowledged
// strictly after the record (too late to discard, correctly promoted) leaves
// no residue either: every op exit scrubs its mark.
func forgetAuditOpCancelled(opID string) {
	takeAuditOpCancelled(opID)
}

func auditErrorf(format string, args ...interface{}) error {
	return errors.New(sanitizeUIText(fmt.Sprintf(format, args...), auditUIMessageMax))
}

func sanitizeAuditID(s string) string {
	return sanitizeUIText(s, auditUIIDMax)
}

func sanitizeAuditPath(s string) string {
	return sanitizeUIText(s, auditUIPathMax)
}

func sanitizeAuditMessage(s string) string {
	return sanitizeUIText(s, auditUIMessageMax)
}

func sanitizeAuditStatus(s string) string {
	return sanitizeUIText(s, auditUIStatusMax)
}

func emitAuditProgress(p Platform, payload AuditProgressPayload) {
	if p == nil {
		return
	}
	p.Emit(EventAuditProgress, payload)
}

// RunIntegrityAudit runs the container-free integrity audit for a project
// (empty projectRoot means the working directory) and returns its sanitized
// result. Findings are a failed result, not a call error: status failed plus
// the mapped findings, nil error. Only cancellation, refused resolution, and
// infrastructure failures (no persisted run) return an error. The run
// registers an op (kind "audit") streaming audit:progress events and
// cancellable via CancelAudit; a cancelled run records nothing, so the
// banner keeps its last completed result. A completed run is recorded under
// the normalized project root and persisted to disk, so LastAuditResult
// survives service restarts.
func RunIntegrityAudit(p Platform, projectRoot string) (AuditResultView, error) {
	opID := nextAuditOpID("audit")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Deferred order is FinishOp then the scrub (LIFO): an op that recorded
	// before a late cancel still exits mark-free.
	defer forgetAuditOpCancelled(opID)
	// The op carries its normalized project scope so the panel can recover
	// the in-flight op id on mount (RunningAuditOpIDForProject) without
	// ever seeing another scope's run. The progress scope below is the same
	// normalized root, so every audit:progress event of this run carries the
	// banner's key identity at emit time.
	RegisterOp(opID, normalizeAuditProjectRoot(projectRoot), "audit", cancel)
	defer FinishOp(opID)

	progressScope := normalizeAuditProjectRoot(projectRoot)

	emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseRunning, Percent: 0, Message: "Running integrity audit...", ProjectRoot: progressScope})

	resolved, err := resolveAuditRunRequestForDesktop(strings.TrimSpace(projectRoot))
	if err != nil {
		emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseFailed, Percent: 100, Message: "Integrity audit failed to start", ProjectRoot: progressScope})
		return AuditResultView{}, auditErrorf("integrity audit: %v", err)
	}
	// Docker-free by construction: the desktop asks for exactly the
	// container-free check; lint and profiler stay disabled rows.
	resolved.Checks = []string{audit.IntegrityCheck}

	result, runErr := runAuditRunnerForDesktop(ctx, resolved)
	if cancelled := ctx.Err() != nil; cancelled {
		// Cancellation is a request, not a kill: the in-flight host analysis
		// keeps running to completion in the background (analyzers take no
		// context), but this late completion is DISCARDED here — never
		// recorded, never promoted to the banner. The runner's partial result
		// stays on disk for explicit reads; the banner keeps its last
		// completed run. The mark is consumed so the cancelled-set stays
		// bounded by in-flight ops.
		takeAuditOpCancelled(opID)
		emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseCancelled, Percent: 100, Message: "Cancelling integrity audit; the in-flight result will be discarded", ProjectRoot: progressScope})
		return AuditResultView{}, auditErrorf("integrity audit cancellation requested: the in-flight analysis keeps running in the background and its result will be discarded")
	}
	if strings.TrimSpace(result.RunID) == "" {
		emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseFailed, Percent: 100, Message: "Integrity audit failed", ProjectRoot: progressScope})
		if runErr != nil {
			return AuditResultView{}, auditErrorf("integrity audit failed: %v", runErr)
		}
		return AuditResultView{}, auditErrorf("integrity audit produced no result")
	}
	view := auditResultView(result, opID)
	if runErr != nil && len(view.Findings) == 0 {
		// Infrastructure failure with no findings to show: an error, not a
		// result. (Findings plus a runner error is the normal
		// findings-failure shape and returns the view with nil error.)
		emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseFailed, Percent: 100, Message: "Integrity audit failed", SessionID: sanitizeAuditID(result.SessionID), ProjectRoot: progressScope})
		return AuditResultView{}, auditErrorf("integrity audit failed: %v", runErr)
	}
	recordRoot := strings.TrimSpace(resolved.ProjectRoot)
	if recordRoot == "" {
		recordRoot = strings.TrimSpace(projectRoot)
	}
	if hook := auditPreRecordHookForDesktop; hook != nil {
		hook(opID)
	}
	recorded, wasCancelled := recordLastAuditResultUnlessCancelled(recordRoot, view, opID)
	if !recorded {
		// The record and the cancelled re-check share the banner-index lock,
		// so a cancel delivered after the early completion check above still
		// lands here: the late completion is discarded, never promoted to
		// the banner. The event states the actual reason: cancelled only
		// when actually cancelled, neutral otherwise.
		if wasCancelled {
			emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseCancelled, Percent: 100, Message: "Cancelling integrity audit; the in-flight result will be discarded", ProjectRoot: progressScope})
			return AuditResultView{}, auditErrorf("integrity audit cancellation requested: the in-flight analysis keeps running in the background and its result will be discarded")
		}
		emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseFailed, Percent: 100, Message: "Integrity audit finished but its result could not be recorded", SessionID: sanitizeAuditID(result.SessionID), ProjectRoot: progressScope})
		return AuditResultView{}, auditErrorf("integrity audit completed but its result could not be recorded")
	}
	emitAuditProgress(p, AuditProgressPayload{OpID: opID, Phase: auditPhaseDone, Percent: 100, Message: "Integrity audit finished", SessionID: view.SessionID, ProjectRoot: progressScope})
	return view, nil
}

// RunningAuditOpIDForProject reports the id of the in-flight integrity
// audit for a project scope, or "" when none is running. Both sides go
// through normalizeAuditProjectRoot, so any spelling of one scope matches
// while another scope never does. It is read-only over the op registry: no
// registration, no cancellation, no side effects.
func RunningAuditOpIDForProject(projectRoot string) string {
	want := normalizeAuditProjectRoot(projectRoot)
	if want == "" {
		return ""
	}
	for _, op := range ListOps() {
		if op.Kind != "audit" {
			continue
		}
		if normalizeAuditProjectRoot(op.Project) == want {
			return op.ID
		}
	}
	return ""
}

// GetAuditResult reads one persisted audit run for a project. An empty runID
// resolves to the session's latest run. It renders the same view as a fresh
// run (integrity job plus disabled docker rows), so the banner and the
// result panel share one shape.
func GetAuditResult(projectID, sessionID, runID string) (AuditResultView, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || sessionID == "" {
		return AuditResultView{}, auditErrorf("audit result requires a project and a session")
	}
	result, err := readAuditRunResultForDesktop(projectID, sessionID, runID)
	if err != nil {
		return AuditResultView{}, auditErrorf("audit result: %v", err)
	}
	if strings.TrimSpace(result.RunID) == "" {
		return AuditResultView{}, auditErrorf("audit run not found")
	}
	return auditResultView(result, ""), nil
}

// LastAuditResult reports the last completed integrity result for a project
// root (any spelling; empty means the working directory), for banner use by
// Task 3. It reads the persisted disk copy, so it survives service restarts.
// It reports false when no run has completed yet; cancelled and
// infrastructure-failed runs never create an entry, so the banner keeps its
// last completed result instead of flipping to an empty state.
func LastAuditResult(projectRoot string) (AuditResultView, bool) {
	key := normalizeAuditProjectRoot(projectRoot)
	if key == "" {
		return AuditResultView{}, false
	}
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	ensureLastAuditResultsLoadedLocked()
	view, ok := lastAuditResultsCache.byRoot[key]
	return view, ok
}

// CancelAudit requests cancellation of a still-running integrity audit. It
// marks the op under the banner-index lock FIRST, then cancels via the op
// registry (CancelOp fires its context cancel and removes the entry): the
// mark makes every later record critical section discard, which is what
// closes the check-then-record race — both sides serialize on the one banner
// mutex. A failed CancelOp cleans a mark this call added unless a concurrent
// success armed it, so the cancelled-set stays bounded by in-flight ops.
// This is a request, not a kill — the in-flight host analysis runs to
// completion in the background (analyzers take no context) and RunIntegrityAudit
// discards its late result instead of promoting it to the banner. Cancelling
// a finished or unknown id is a harmless no-op reporting false.
func CancelAudit(opID string) bool {
	id := strings.TrimSpace(opID)
	newlyMarked := markAuditOpCancelled(id)
	ok := CancelOp(id)
	settleAuditOpCancelled(id, ok, newlyMarked)
	return ok
}

// auditResultView renders a persisted or just-completed run result. The
// integrity job maps to findings; lint and profiler are always present as
// disabled rows with reason, never hidden.
func auditResultView(result audit.RunResult, opID string) AuditResultView {
	view := AuditResultView{
		ProjectID:  sanitizeAuditID(result.ProjectID),
		SessionID:  sanitizeAuditID(result.SessionID),
		RunID:      sanitizeAuditID(result.RunID),
		Status:     sanitizeAuditStatus(string(result.Status)),
		FinishedAt: result.FinishedAt,
		OpID:       sanitizeAuditID(opID),
		Checks: []AuditCheckView{
			{ID: audit.IntegrityCheck, Title: "Integrity (container-free)", Enabled: true},
			{ID: "lint", Title: "Lint", Enabled: false, DisabledReason: auditDockerDisabledReason + ": run `govard audit run --checks lint` from the CLI"},
			{ID: "profiler", Title: "Profiler", Enabled: false, DisabledReason: auditDockerDisabledReason + ": run `govard audit run --checks profiler` from the CLI"},
		},
	}
	integrityFound := false
	for _, job := range result.Jobs {
		if strings.TrimSpace(job.ID) != audit.IntegrityCheck {
			continue
		}
		integrityFound = true
		view.Findings = auditFindingsFromEvidence(job.Evidence)
		break
	}
	if !integrityFound {
		view.Checks[0] = AuditCheckView{ID: audit.IntegrityCheck, Title: "Integrity (container-free)", Enabled: false, DisabledReason: "Session has no integrity run"}
	}
	view.FindingCount = len(view.Findings)
	return view
}

// auditFindingsFromEvidence extracts integrity findings from a job's evidence
// map. In-memory results carry []audit.LintFinding; results re-read from the
// audit store (JSON round-trip) carry []any of string maps. Anything else is
// ignored rather than misrendered.
func auditFindingsFromEvidence(evidence map[string]any) []AuditFindingView {
	if len(evidence) == 0 {
		return nil
	}
	raw, ok := evidence["findings"]
	if !ok {
		return nil
	}
	if findings, ok := raw.([]audit.LintFinding); ok {
		return sanitizeAuditFindings(findings)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	findings := make([]audit.LintFinding, 0, len(items))
	for _, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		finding := audit.LintFinding{
			Tool:    auditEvidenceString(fields, "tool"),
			Rule:    auditEvidenceString(fields, "rule"),
			Path:    auditEvidenceString(fields, "path"),
			Message: auditEvidenceString(fields, "message"),
		}
		if line, ok := fields["line"].(float64); ok {
			finding.Line = int(line)
		}
		findings = append(findings, finding)
	}
	return sanitizeAuditFindings(findings)
}

func auditEvidenceString(fields map[string]any, key string) string {
	value, ok := fields[key].(string)
	if !ok {
		return ""
	}
	return value
}

func sanitizeAuditFindings(findings []audit.LintFinding) []AuditFindingView {
	views := make([]AuditFindingView, 0, len(findings))
	for _, finding := range findings {
		views = append(views, AuditFindingView{
			Tool:    sanitizeAuditID(finding.Tool),
			Rule:    sanitizeAuditID(finding.Rule),
			Path:    sanitizeAuditPath(finding.Path),
			Line:    finding.Line,
			Message: sanitizeAuditMessage(finding.Message),
		})
	}
	return views
}

// defaultResolveAuditRunRequest resolves the docker-free audit context from
// the project root (empty means the working directory). It wraps the
// framework-owned target resolver and the project config the same way the
// CLI's audit preparation does, minus everything container-backed: no lint
// provider, no PHP policy probe, no profiler runtime. A framework without an
// integrity profile is refused here, before any runner exists.
func defaultResolveAuditRunRequest(projectRoot string) (audit.RunRequest, error) {
	start := strings.TrimSpace(projectRoot)
	if start == "" {
		workdir, err := os.Getwd()
		if err != nil {
			return audit.RunRequest{}, fmt.Errorf("resolve working directory: %w", err)
		}
		start = workdir
	}
	definition, target, err := frameworks.ResolveAuditTarget(types.AuditTargetResolveRequest{StartPath: start, ModeOverride: types.AuditTargetAuto})
	if err != nil {
		return audit.RunRequest{}, err
	}
	if definition.AuditIntegrity == nil {
		return audit.RunRequest{}, fmt.Errorf("framework %q does not support integrity audit", definition.Name)
	}
	root := strings.TrimSpace(target.ProjectRoot)
	if root == "" {
		root = strings.TrimSpace(target.TargetPath)
	}
	if root == "" {
		root = start
	}
	var config *engine.Config
	if target.Mode != types.AuditTargetStandalone {
		loaded, loadErr := engine.LoadRawConfigFromDir(root, true)
		if loadErr != nil {
			return audit.RunRequest{}, fmt.Errorf("load audit project config: %w", loadErr)
		}
		config = &loaded
	}
	canonicalRoot, canonicalErr := filepath.Abs(root)
	if canonicalErr != nil {
		canonicalRoot = root
	} else if resolved, evalErr := filepath.EvalSymlinks(canonicalRoot); evalErr == nil {
		canonicalRoot = resolved
	}
	origin, _ := auditGitOutput(root, "config", "--get", "remote.origin.url")
	environment := audit.EnvironmentFingerprint{Framework: definition.Name, GovardVersion: Version}
	stackPHP := ""
	if config != nil {
		environment.FrameworkVersion = config.FrameworkVersion
		environment.WebServer = config.Stack.Services.WebServer
		stackPHP = config.Stack.PHPVersion
	}
	return audit.RunRequest{
		ProjectRoot:      root,
		ProjectID:        audit.ProjectID(canonicalRoot, projectstores.RepositoryIdentity(canonicalRoot, origin)),
		Scope:            audit.ScopeProject,
		Environment:      environment,
		Source:           auditSourceFingerprint(root),
		IntegrityProfile: *definition.AuditIntegrity,
		StackPHPVersion:  stackPHP,
		Target:           target,
	}, nil
}

// defaultRunAuditRunner executes the integrity-only request against a
// store-backed runner. The runner builds just the host-analysis job for the
// integrity check, so no lint backend, profiler runtime, or container is
// ever touched.
func defaultRunAuditRunner(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
	store := audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir()))
	return audit.NewRunner(audit.RunnerOptions{Store: store}).Run(ctx, request)
}

// defaultReadAuditRunResult reads a persisted run from the audit store. An
// empty runID resolves to the session's latest run.
func defaultReadAuditRunResult(projectID, sessionID, runID string) (audit.RunResult, error) {
	store := audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir()))
	manifest, err := store.ReadSession(projectID, sessionID)
	if err != nil {
		return audit.RunResult{}, err
	}
	if strings.TrimSpace(runID) == "" {
		if len(manifest.Runs) == 0 {
			return audit.RunResult{}, fmt.Errorf("audit session %q has no runs", sessionID)
		}
		runID = manifest.Runs[len(manifest.Runs)-1].RunID
	}
	return store.ReadResult(projectID, sessionID, runID)
}

// auditSourceFingerprint mirrors the CLI's manifest digest plus best-effort
// git identity: the runner records it on the session, never gates on it.
func auditSourceFingerprint(root string) audit.SourceFingerprint {
	digest := auditManifestDigest(root)
	commit, commitErr := auditGitOutput(root, "rev-parse", "HEAD")
	if commitErr != nil {
		return audit.SourceFingerprint{Digest: digest}
	}
	dirty := false
	if err := exec.Command("git", "-C", root, "diff", "--quiet").Run(); err != nil {
		dirty = true
	}
	return audit.SourceFingerprint{GitCommit: commit, GitDirty: dirty, Digest: digest}
}

// auditManifestDigest hashes the project manifest files the CLI fingerprints
// for its audit source identity.
func auditManifestDigest(root string) string {
	hash := sha256.New()
	for _, name := range []string{".govard.yml", "composer.json", "composer.lock"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		_, _ = io.WriteString(hash, name+"\n")
		_, _ = hash.Write(content)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func auditGitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
