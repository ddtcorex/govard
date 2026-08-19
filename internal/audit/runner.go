package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	SelectedPHPVersions []string
	MatrixComplete      bool
	// BypassResultCache asks the lint backend to ignore reusable analyzer
	// state for this run.
	BypassResultCache bool
}

type RunnerOptions struct {
	Store       *Store
	LintBackend LintBackend
	// LintCacheRoot is the reusable lint cache root (see
	// DefaultLintCacheRoot). When empty, the cache stays beside the persisted
	// sessions of the audited project.
	LintCacheRoot string
	Resources     Resources
	Now           func() time.Time
}

// Runner coordinates persisted audit sessions without knowing any framework.
type Runner struct {
	store         *Store
	lintBackend   LintBackend
	lintCacheRoot string
	resources     Resources
	now           func() time.Time
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
	return &Runner{store: options.Store, lintBackend: options.LintBackend, lintCacheRoot: options.LintCacheRoot, resources: resources, now: now}
}

func (runner *Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if err := runner.validateRequest(request); err != nil {
		return RunResult{}, err
	}
	checks, err := normalizeChecks(request.Checks)
	if err != nil {
		return RunResult{}, err
	}
	request.Checks = checks
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
// run. Scope, base ref, project root, environment, source and lint policy are
// reloaded from the persisted session/previous run rather than caller input.
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
	checks, err = normalizeChecks(checks)
	if err != nil {
		return RunResult{}, err
	}
	previous, err := runner.store.ReadResult(projectID, sessionID, manifest.Runs[len(manifest.Runs)-1].RunID)
	if err != nil {
		return RunResult{}, err
	}
	settings, err := persistedLintSettings(previous)
	if err != nil {
		return RunResult{}, err
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
		Target:              savedLintTarget(settings.Target, manifest),
		SelectedPHPVersions: settings.SelectedPHPVersions,
		MatrixComplete:      settings.MatrixComplete,
	})
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
	if runner.lintBackend == nil {
		return nil, errors.New("audit runner lint backend is not configured")
	}
	sessionPath := runner.store.SessionPath(manifest.ProjectID, manifest.SessionID)
	if sessionPath == "" {
		return nil, errors.New("resolve audit session path")
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
	return []Job{{ID: "lint", Kind: "static-analysis", Resources: Resources{CPU: request.LintJobs, MemoryMB: 2048}, Run: lintJob}}, nil
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
	if runner.lintBackend == nil {
		return errors.New("audit runner lint backend is not configured")
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
	return nil
}

func normalizeChecks(checks []string) ([]string, error) {
	if len(checks) == 0 {
		return []string{"lint"}, nil
	}
	seen := make(map[string]bool, len(checks))
	normalized := make([]string, 0, len(checks))
	for _, check := range checks {
		check = strings.TrimSpace(check)
		if check != "lint" {
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
	return savedLintSettings{}, errors.New("audit session has no persisted lint settings")
}
