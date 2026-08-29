package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

// RunRequest describes one explicit audit execution. The session manifest
// captures the immutable project/source/scope inputs before work starts.
type RunRequest struct {
	ProjectRoot         string
	ProjectID           string
	Scope               Scope
	BaseRef             string
	Checks              []string
	LintJobs            int
	Environment         EnvironmentFingerprint
	Source              SourceFingerprint
	LintProfile         types.AuditLintProfile
	Target              types.AuditTarget
	ProfilerURL         string
	SelectedPHPVersions []string
	MatrixComplete      bool
	// BypassResultCache asks the lint backend to ignore reusable analyzer
	// state for this run.
	BypassResultCache bool
	// Output receives live progress while the run executes when the command
	// is in text mode on an interactive terminal. The runner forwards it to
	// the lint backend's StreamWriter so Docker logs (phase start/end,
	// per-PHP cache state, and later per-finding lines) appear as they
	// happen, mirroring vendor/bin/phpcs/phpstan. In --format json the
	// field stays nil so machine output is not polluted.
	Output io.Writer
}

type RunnerOptions struct {
	Store                  *Store
	LintBackend            LintBackend
	ProfilerRuntime        ProfilerRuntime
	ProfilerCleanupTimeout time.Duration
	// LintCacheRoot is the reusable lint cache root (see
	// DefaultLintCacheRoot). When empty, the cache stays beside the persisted
	// sessions of the audited project.
	LintCacheRoot string
	Resources     Resources
	Now           func() time.Time
}

// Runner coordinates persisted audit sessions without knowing any framework.
type Runner struct {
	store                  *Store
	lintBackend            LintBackend
	profilerRuntime        ProfilerRuntime
	profilerCleanupTimeout time.Duration
	lintCacheRoot          string
	resources              Resources
	now                    func() time.Time
}

func NewRunner(options RunnerOptions) *Runner {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	resources := options.Resources
	if resources.CPU == 0 {
		resources.CPU = 1
	}
	if resources.MemoryMB == 0 {
		resources.MemoryMB = 2048
	}
	profilerCleanupTimeout := options.ProfilerCleanupTimeout
	if profilerCleanupTimeout <= 0 {
		profilerCleanupTimeout = defaultProfilerCleanupTimeout
	}
	return &Runner{store: options.Store, lintBackend: options.LintBackend, profilerRuntime: options.ProfilerRuntime, profilerCleanupTimeout: profilerCleanupTimeout, lintCacheRoot: options.LintCacheRoot, resources: resources, now: now}
}

func (runner *Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	checks, err := NormalizeChecks(request.Checks)
	if err != nil {
		return RunResult{}, err
	}
	request.Checks = checks
	if err := runner.validateRequest(request); err != nil {
		return RunResult{}, err
	}
	manifest, err := runner.store.CreateSession(SessionManifest{
		SchemaVersion: SchemaVersion,
		ProjectID:     request.ProjectID,
		ProjectRoot:   request.ProjectRoot,
		Scope:         request.Scope,
		BaseRef:       request.BaseRef,
		Environment:   request.Environment,
		Source:        request.Source,
		Runs:          []RunReference{},
	})
	if err != nil {
		return RunResult{}, err
	}
	return runner.runInSession(ctx, manifest, request)
}

// Rerun always requires a caller-selected session; it never guesses a latest
// run. Scope, base ref, project root, environment, source, lint policy, and —
// when the caller passes no checks — the check selection are reloaded from the
// persisted session/previous run rather than caller input.
func (runner *Runner) Rerun(ctx context.Context, sessionID, projectID string, checks []string) (RunResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return RunResult{}, errors.New("audit rerun requires an explicit session ID")
	}
	if runner == nil || runner.store == nil {
		return RunResult{}, errors.New("audit runner store is not configured")
	}
	manifest, err := runner.store.ReadSession(projectID, sessionID)
	if err != nil {
		return RunResult{}, err
	}
	if manifest.ProjectID != projectID {
		return RunResult{}, fmt.Errorf("audit session %q belongs to project %q", sessionID, manifest.ProjectID)
	}
	if len(manifest.Runs) == 0 {
		return RunResult{}, fmt.Errorf("audit session %q has no prior run to rerun", sessionID)
	}
	// An omitted --checks repeats the latest run's selection so a rerun of a
	// profiler-only session does not silently demand lint settings it never had.
	if len(checks) == 0 {
		checks, err = runner.latestRunChecks(projectID, sessionID, manifest.Runs)
		if err != nil {
			return RunResult{}, err
		}
	}
	checks, err = NormalizeChecks(checks)
	if err != nil {
		return RunResult{}, err
	}
	settings := savedLintSettings{}
	target := types.AuditTarget{}
	requestURL := ""
	if includesCheck(checks, "lint") {
		settings, err = runner.latestLintSettings(projectID, sessionID, manifest.Runs)
		if err != nil {
			return RunResult{}, err
		}
		target = savedLintTarget(settings.Target, manifest)
	}
	if includesCheck(checks, "profiler") {
		profilerSettings, err := runner.latestProfilerSettings(projectID, sessionID, manifest.Runs)
		if err != nil {
			return RunResult{}, err
		}
		if target == (types.AuditTarget{}) {
			target = profilerSettings.Target
		}
		requestURL = profilerSettings.URL
	}
	return runner.runInSession(ctx, manifest, RunRequest{
		ProjectRoot:         manifest.ProjectRoot,
		ProjectID:           manifest.ProjectID,
		Scope:               manifest.Scope,
		BaseRef:             manifest.BaseRef,
		Checks:              checks,
		LintJobs:            settings.Jobs,
		Environment:         manifest.Environment,
		Source:              manifest.Source,
		LintProfile:         settings.Profile,
		Target:              target,
		ProfilerURL:         requestURL,
		SelectedPHPVersions: settings.SelectedPHPVersions,
		MatrixComplete:      settings.MatrixComplete,
	})
}

// LatestRunChecks reports the check selection persisted in the newest run of
// an explicit session. The rerun command uses it to construct only the
// backends that selection actually needs before starting a rerun.
func (runner *Runner) LatestRunChecks(_ context.Context, projectID, sessionID string) ([]string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("audit session checks lookup requires an explicit session ID")
	}
	if runner == nil || runner.store == nil {
		return nil, errors.New("audit runner store is not configured")
	}
	manifest, err := runner.store.ReadSession(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	if len(manifest.Runs) == 0 {
		return nil, fmt.Errorf("audit session %q has no prior run to rerun", sessionID)
	}
	return runner.latestRunChecks(projectID, sessionID, manifest.Runs)
}

func (runner *Runner) latestRunChecks(projectID, sessionID string, runs []RunReference) ([]string, error) {
	for index := len(runs) - 1; index >= 0; index-- {
		result, err := runner.store.ReadResult(projectID, sessionID, runs[index].RunID)
		if err != nil {
			return nil, err
		}
		checks := make([]string, 0, 2)
		for _, job := range result.Jobs {
			if job.ID == "lint" || job.ID == "profiler" {
				checks = append(checks, job.ID)
			}
		}
		if len(checks) > 0 {
			return checks, nil
		}
	}
	return nil, fmt.Errorf("audit session %q has no run with recognized checks", sessionID)
}

func (runner *Runner) Status(projectID, sessionID string) (SessionManifest, error) {
	if strings.TrimSpace(sessionID) == "" {
		return SessionManifest{}, errors.New("audit status requires an explicit session ID")
	}
	if runner == nil || runner.store == nil {
		return SessionManifest{}, errors.New("audit runner store is not configured")
	}
	return runner.store.ReadSession(projectID, sessionID)
}

func (runner *Runner) Result(projectID, sessionID, runID string) (RunResult, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(runID) == "" {
		return RunResult{}, errors.New("audit result requires explicit session and run IDs")
	}
	if runner == nil || runner.store == nil {
		return RunResult{}, errors.New("audit runner store is not configured")
	}
	return runner.store.ReadResult(projectID, sessionID, runID)
}

// CleanupOlderThan removes persisted sessions without touching reusable lint
// caches. Keeping this on Runner ensures CLI dependency injection and cleanup
// use the same store.
func (runner *Runner) CleanupOlderThan(projectID string, cutoff time.Time) ([]string, error) {
	if runner == nil || runner.store == nil {
		return nil, errors.New("audit runner store is not configured")
	}
	return runner.store.CleanupOlderThan(projectID, cutoff)
}

func (runner *Runner) runInSession(ctx context.Context, manifest SessionManifest, request RunRequest) (RunResult, error) {
	run, err := runner.store.CreateRun(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{
		SchemaVersion: SchemaVersion,
		SessionID:     manifest.SessionID,
		RunID:         run.RunID,
		ProjectID:     manifest.ProjectID,
		Scope:         manifest.Scope,
		Status:        StatusPending,
		Environment:   manifest.Environment,
		Source:        manifest.Source,
		Jobs:          []JobResult{},
		Artifacts:     []Artifact{},
		Errors:        []AuditError{},
	}
	if err := runner.store.WriteResult(result); err != nil {
		return result, err
	}

	jobs, err := runner.jobsFor(request, manifest, run.RunID)
	if err == nil {
		jobsResult, scheduleErr := NewScheduler(runner.resources).Run(ctx, jobs)
		result.Jobs = jobsResult
		result.Artifacts = append(result.Artifacts, profilerArtifacts(jobsResult)...)
		err = scheduleErr
		if err == nil {
			err = infrastructureError(jobsResult)
		}
	}
	result.StartedAt = runner.now().UTC()
	result.FinishedAt = runner.now().UTC()
	result.Status = resultStatus(result.Jobs, err, ctx)
	if err != nil {
		result.Errors = append(result.Errors, AuditError{Code: "infrastructure", Message: err.Error()})
	}
	if writeErr := runner.store.WriteResult(result); writeErr != nil {
		if err != nil {
			return result, fmt.Errorf("%w; persist terminal audit result: %v", err, writeErr)
		}
		return result, writeErr
	}
	return result, err
}

func (runner *Runner) jobsFor(request RunRequest, manifest SessionManifest, runID string) ([]Job, error) {
	if runner == nil || runner.store == nil {
		return nil, errors.New("audit runner store is not configured")
	}
	jobs := make([]Job, 0, len(request.Checks))
	for _, check := range request.Checks {
		switch check {
		case "lint":
			lintJob, err := runner.lintJob(request, manifest, runID)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, lintJob)
		case "profiler":
			if runner.profilerRuntime == nil {
				return nil, errors.New("audit runner profiler runtime is not configured")
			}
			profilerRequest := ProfilerRequest{
				ProjectRoot: request.ProjectRoot,
				ProjectID:   manifest.ProjectID,
				SessionID:   manifest.SessionID,
				RunID:       runID,
				URL:         request.ProfilerURL,
				Target:      request.Target,
			}
			jobs = append(jobs, Job{ID: "profiler", Kind: "runtime-profiler", Resources: Resources{CPU: 1, MemoryMB: 256}, Run: runner.profilerJob(profilerRequest)})
		}
	}
	return jobs, nil
}

func (runner *Runner) lintJob(request RunRequest, manifest SessionManifest, runID string) (Job, error) {
	if runner.lintBackend == nil {
		return Job{}, errors.New("audit runner lint backend is not configured")
	}
	sessionPath := runner.store.SessionPath(manifest.ProjectID, manifest.SessionID)
	if sessionPath == "" {
		return Job{}, errors.New("resolve audit session path")
	}
	runDir := filepath.Join(sessionPath, "runs", runID)
	cacheRoot := runner.lintCacheRoot
	if cacheRoot == "" {
		cacheRoot = filepath.Join(filepath.Dir(filepath.Dir(sessionPath)), "lint-cache")
	}
	lintRequest := LintRequest{
		ProjectRoot:         request.ProjectRoot,
		ProjectID:           request.ProjectID,
		SessionID:           manifest.SessionID,
		RunID:               runID,
		Provider:            runner.lintBackend.Name(),
		TargetID:            lintTargetID(manifest.ProjectID, request.Target),
		Target:              request.Target,
		RunDir:              runDir,
		CacheRoot:           cacheRoot,
		Scope:               request.Scope,
		BaseRef:             request.BaseRef,
		Jobs:                request.LintJobs,
		Profile:             request.LintProfile,
		SelectedPHPVersions: append([]string(nil), request.SelectedPHPVersions...),
		MatrixComplete:      request.MatrixComplete,
		BypassResultCache:   request.BypassResultCache,
		StreamWriter:        request.Output,
	}
	lintJob := func(ctx context.Context) (map[string]any, error) {
		report, runErr := runner.lintBackend.Run(ctx, lintRequest)
		evidence := lintEvidence(report, request)
		if runErr != nil {
			// A backend that stopped because the run was cancelled is not an
			// infrastructure failure, so it must not add a structured audit
			// error; the cancelled status carries the outcome on its own.
			if report.Status != lintStatusCancelled {
				evidence["infrastructure_error"] = runErr.Error()
			}
			return evidence, runErr
		}
		if err := validateLintReportAggregate(report); err != nil {
			evidence["infrastructure_error"] = err.Error()
			return evidence, err
		}
		// Host-side media guard: every lint run also enforces pub/media hygiene
		// via a name-only scan (M2-LINT-MEDIA). This mirrors the container's
		// media-guard phase so docs claiming "every lint run enforces media
		// guard" hold even for external providers or when the container phase
		// is skipped. It also provides Go-native enforcement for tests.
		if findings := engine.ScanMediaGuard(request.ProjectRoot); len(findings) > 0 {
			mediaEvidence := make([]map[string]any, 0, len(findings))
			for _, f := range findings {
				mediaEvidence = append(mediaEvidence, map[string]any{
					"path":    f.Path,
					"tool":    "M2-LINT-MEDIA",
					"rule":    "M2-LINT-MEDIA",
					"message": "PHP file in pub/media",
				})
			}
			evidence["media_guard"] = map[string]any{
				"status":   "failed",
				"findings": mediaEvidence,
			}
			// If the backend already reported failed, keep that status but
			// surface the host findings as well; if it passed, force failure.
			if report.Status == lintStatusPassed {
				evidence["lint_status"] = lintStatusFailed
			}
			// Preserve cancelled/infra_error as-is; only passed/failed are
			// eligible for media-guard failure injection.
			if report.Status == lintStatusPassed || report.Status == lintStatusFailed {
				return evidence, lintFailureError{}
			}
		} else {
			evidence["media_guard"] = map[string]any{"status": "passed"}
		}
		switch report.Status {
		case lintStatusInfraError:
			// The backend published a report but the analysis itself never
			// completed. That is an infrastructure failure exactly like a
			// Go-level backend error, not a lint outcome.
			err := fmt.Errorf("lint reported an infrastructure error: %s", lintInfrastructureReason(report))
			evidence["infrastructure_error"] = err.Error()
			return evidence, err
		case lintStatusCancelled:
			// A cancelled report is surfaced through the same cancelled-status
			// path as a cancelled context, never as an infrastructure failure.
			return evidence, lintCancelledError{}
		case lintStatusFailed:
			return evidence, lintFailureError{}
		}
		return evidence, nil
	}
	return Job{ID: "lint", Kind: "static-analysis", Resources: Resources{CPU: request.LintJobs, MemoryMB: 2048}, Run: lintJob}, nil
}

// lintTargetID derives a stable per-target identifier from the canonical
// target identity. Hashing keeps literal source paths out of cache directory
// names, and including the mode and target path keeps a project, a module
// inside it, and a standalone module in distinct namespaces.
func lintTargetID(projectID string, target types.AuditTarget) string {
	identity := strings.Join([]string{projectID, string(target.Mode), target.TargetPath}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "target-" + hex.EncodeToString(sum[:])[:32]
}

// lintInfrastructureReason summarizes why analysis could not complete, using
// only the evidence the report already carries.
func lintInfrastructureReason(report LintReport) string {
	for _, result := range report.PHPResults {
		if result.Outcome != lintStatusInfraError {
			continue
		}
		for _, phase := range result.Phases {
			if phase.Status == "error" && phase.CacheReason != "" {
				return fmt.Sprintf("PHP %s %s: %s", result.PHPVersion, phase.Name, phase.CacheReason)
			}
		}
		if result.Cache.Reason != "" {
			return fmt.Sprintf("PHP %s: %s", result.PHPVersion, result.Cache.Reason)
		}
		return "PHP " + result.PHPVersion
	}
	return "no PHP result carries the failing phase"
}

func infrastructureError(jobs []JobResult) error {
	for _, job := range jobs {
		if message, ok := job.Evidence["infrastructure_error"].(string); ok && message != "" {
			return errors.New(message)
		}
	}
	return nil
}

type lintFailureError struct{}

func (lintFailureError) Error() string { return "lint reported failed checks" }

type lintCancelledError struct{}

func (lintCancelledError) Error() string { return "lint reported a cancelled run" }

// cancelledLintJob reports whether a lint job's own evidence says the run was
// cancelled. It mirrors how infrastructureError reads job evidence, so a
// container cancelled without this process being cancelled still terminates
// the run as cancelled instead of as a generic failure.
func cancelledLintJob(jobs []JobResult) bool {
	for _, job := range jobs {
		if status, ok := job.Evidence["lint_status"].(string); ok && status == lintStatusCancelled {
			return true
		}
	}
	return false
}

func (runner *Runner) validateRequest(request RunRequest) error {
	if runner == nil || runner.store == nil {
		return errors.New("audit runner store is not configured")
	}
	if includesCheck(request.Checks, "lint") && runner.lintBackend == nil {
		return errors.New("audit runner lint backend is not configured")
	}
	if includesCheck(request.Checks, "profiler") && runner.profilerRuntime == nil {
		return errors.New("audit runner profiler runtime is not configured")
	}
	if includesCheck(request.Checks, "profiler") {
		if err := ValidateProfilerURL(request.ProfilerURL); err != nil {
			return err
		}
		if request.Target.Mode == types.AuditTargetStandalone {
			return errors.New("audit profiler does not support standalone targets; run it from a Govard project")
		}
	}
	if strings.TrimSpace(request.ProjectRoot) == "" || strings.TrimSpace(request.ProjectID) == "" {
		return errors.New("audit project root and project ID are required")
	}
	if request.Scope != ScopeProject && request.Scope != ScopeDiff {
		return fmt.Errorf("unsupported audit scope %q", request.Scope)
	}
	if request.Scope == ScopeDiff && strings.TrimSpace(request.BaseRef) == "" {
		return errors.New("diff audit requires a base ref")
	}
	if includesCheck(request.Checks, "lint") {
		if request.LintJobs < 1 {
			return errors.New("lint jobs must be at least one")
		}
		if err := validateLintMatrix(LintRequest{
			Profile:             request.LintProfile,
			Target:              request.Target,
			SelectedPHPVersions: request.SelectedPHPVersions,
			MatrixComplete:      request.MatrixComplete,
		}); err != nil {
			return err
		}
	}
	return nil
}

func includesCheck(checks []string, candidate string) bool {
	for _, check := range checks {
		if check == candidate {
			return true
		}
	}
	return false
}

// NormalizeChecks trims, validates, and deduplicates requested audit checks
// while preserving the caller's first-seen order.
func NormalizeChecks(checks []string) ([]string, error) {
	if len(checks) == 0 {
		return []string{"lint"}, nil
	}
	seen := make(map[string]bool, len(checks))
	normalized := make([]string, 0, len(checks))
	for _, check := range checks {
		check = strings.TrimSpace(check)
		if check != "lint" && check != "profiler" {
			return nil, fmt.Errorf("audit check %q is not implemented", check)
		}
		if !seen[check] {
			seen[check] = true
			normalized = append(normalized, check)
		}
	}
	return normalized, nil
}

func resultStatus(jobs []JobResult, runErr error, ctx context.Context) Status {
	if ctx != nil && ctx.Err() != nil {
		return StatusCancelled
	}
	if cancelledLintJob(jobs) {
		return StatusCancelled
	}
	if runErr != nil {
		return StatusFailed
	}
	for _, job := range jobs {
		if job.Status == StatusCancelled {
			return StatusCancelled
		}
		if job.Status == StatusFailed {
			return StatusFailed
		}
	}
	return StatusPassed
}

type savedLintSettings struct {
	Profile             types.AuditLintProfile `json:"profile"`
	Jobs                int                    `json:"jobs"`
	Target              types.AuditTarget      `json:"target"`
	SelectedPHPVersions []string               `json:"selected_php_versions"`
	MatrixComplete      bool                   `json:"matrix_complete"`
}

type savedProfilerSettings struct {
	URL    string            `json:"url"`
	Target types.AuditTarget `json:"target"`
}

var (
	errNoPersistedLintSettings     = errors.New("audit session has no persisted lint settings")
	errNoPersistedProfilerSettings = errors.New("audit session has no persisted profiler settings")
)

type legacySavedLintSettings struct {
	Profile struct {
		PHPVersions []string `json:"PHPVersions"`
	} `json:"profile"`
}

func lintEvidence(report LintReport, request RunRequest) map[string]any {
	return map[string]any{
		"provider":         report.Provider,
		"lint_status":      report.Status,
		"image_digest":     report.ImageDigest,
		"toolchain_digest": report.ToolchainDigest,
		"duration_ms":      report.DurationMS,
		"php_results":      report.PHPResults,
		"effective_scope":  string(ScopeProject),
		"lint_settings": savedLintSettings{
			Profile:             request.LintProfile,
			Jobs:                request.LintJobs,
			Target:              request.Target,
			SelectedPHPVersions: append([]string(nil), request.SelectedPHPVersions...),
			MatrixComplete:      request.MatrixComplete,
		},
	}
}

func savedLintTarget(target types.AuditTarget, manifest SessionManifest) types.AuditTarget {
	if target.Mode != "" && target.TargetPath != "" {
		return target
	}
	return types.AuditTarget{
		Framework:   manifest.Environment.Framework,
		ProjectRoot: manifest.ProjectRoot,
		TargetPath:  manifest.ProjectRoot,
		Mode:        types.AuditTargetProject,
	}
}

func persistedLintSettings(result RunResult) (savedLintSettings, error) {
	for _, job := range result.Jobs {
		if job.ID != "lint" {
			continue
		}
		value, ok := job.Evidence["lint_settings"]
		if !ok {
			break
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return savedLintSettings{}, err
		}
		var settings savedLintSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			return savedLintSettings{}, fmt.Errorf("decode saved lint settings: %w", err)
		}
		if len(settings.SelectedPHPVersions) == 0 {
			var legacy legacySavedLintSettings
			if err := json.Unmarshal(raw, &legacy); err != nil {
				return savedLintSettings{}, fmt.Errorf("decode legacy lint settings: %w", err)
			}
			if len(legacy.Profile.PHPVersions) > 0 {
				settings.Profile.ProjectPHPVersions = append([]string(nil), legacy.Profile.PHPVersions...)
			}
			settings.SelectedPHPVersions = append([]string(nil), settings.Profile.ProjectPHPVersions...)
			settings.MatrixComplete = true
		}
		if settings.Jobs < 1 {
			return savedLintSettings{}, errors.New("saved lint settings have invalid job count")
		}
		return settings, nil
	}
	return savedLintSettings{}, errNoPersistedLintSettings
}

func (runner *Runner) latestLintSettings(projectID, sessionID string, runs []RunReference) (savedLintSettings, error) {
	for index := len(runs) - 1; index >= 0; index-- {
		result, err := runner.store.ReadResult(projectID, sessionID, runs[index].RunID)
		if err != nil {
			return savedLintSettings{}, err
		}
		settings, err := persistedLintSettings(result)
		if errors.Is(err, errNoPersistedLintSettings) {
			continue
		}
		return settings, err
	}
	return savedLintSettings{}, errNoPersistedLintSettings
}

func persistedProfilerSettings(result RunResult) (savedProfilerSettings, error) {
	for _, job := range result.Jobs {
		if job.ID != "profiler" {
			continue
		}
		value, ok := job.Evidence["profiler_settings"]
		if !ok {
			break
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return savedProfilerSettings{}, err
		}
		var settings savedProfilerSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			return savedProfilerSettings{}, fmt.Errorf("decode saved profiler settings: %w", err)
		}
		return settings, nil
	}
	return savedProfilerSettings{}, errNoPersistedProfilerSettings
}

func (runner *Runner) latestProfilerSettings(projectID, sessionID string, runs []RunReference) (savedProfilerSettings, error) {
	for index := len(runs) - 1; index >= 0; index-- {
		result, err := runner.store.ReadResult(projectID, sessionID, runs[index].RunID)
		if err != nil {
			return savedProfilerSettings{}, err
		}
		settings, err := persistedProfilerSettings(result)
		if errors.Is(err, errNoPersistedProfilerSettings) {
			continue
		}
		return settings, err
	}
	return savedProfilerSettings{}, errNoPersistedProfilerSettings
}
