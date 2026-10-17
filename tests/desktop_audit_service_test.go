package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/desktop"
	"govard/internal/frameworks/types"
)

func setupAuditServiceTest(t *testing.T) {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetAuditServiceForTest()
	desktop.ResetOperationRegistryForTest()
	t.Cleanup(desktop.ResetAuditServiceForTest)
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

func stubAuditResolve(t *testing.T, projectID string) {
	t.Helper()
	t.Cleanup(desktop.SetAuditResolveForDesktopForTest(
		func(projectRoot string) (audit.RunRequest, error) {
			return audit.RunRequest{
				ProjectRoot: projectRoot,
				ProjectID:   projectID,
				Scope:       audit.ScopeProject,
			}, nil
		}))
}

func passedIntegrityResult() audit.RunResult {
	return audit.RunResult{
		SchemaVersion: audit.SchemaVersion,
		SessionID:     "sess-1",
		RunID:         "1",
		ProjectID:     "project-test",
		Scope:         audit.ScopeProject,
		Status:        audit.StatusPassed,
		Jobs: []audit.JobResult{
			{ID: audit.IntegrityCheck, Kind: "host-analysis", Status: audit.StatusPassed},
		},
	}
}

func auditCheckByID(t *testing.T, view desktop.AuditResultView, id string) desktop.AuditCheckView {
	t.Helper()
	for _, check := range view.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("AuditResultView has no check %q (checks=%+v)", id, view.Checks)
	return desktop.AuditCheckView{}
}

// An integrity run completes docker-free: the runner is asked for exactly
// the integrity check, the view reports it enabled, and the docker-backed
// checks are present but disabled with a reason (never hidden).
func TestAuditService_IntegrityRunCompletesDockerFree(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	var gotChecks []string
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			gotChecks = append([]string(nil), request.Checks...)
			return passedIntegrityResult(), nil
		}))

	platform := &desktop.FakePlatform{}
	view, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
	if err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	if len(gotChecks) != 1 || gotChecks[0] != audit.IntegrityCheck {
		t.Fatalf("runner checks = %v, want exactly [integrity] (docker-free)", gotChecks)
	}
	if view.SessionID != "sess-1" || view.RunID != "1" || view.ProjectID != "project-test" {
		t.Fatalf("view identity = %+v, want sess-1/1/project-test", view)
	}
	if view.Status != string(audit.StatusPassed) {
		t.Fatalf("view status = %q, want passed", view.Status)
	}
	if view.OpID == "" {
		t.Fatalf("view OpID empty, want a registry op id")
	}

	integrity := auditCheckByID(t, view, audit.IntegrityCheck)
	if !integrity.Enabled {
		t.Fatalf("integrity check %+v, want enabled", integrity)
	}
	for _, id := range []string{"lint", "profiler"} {
		check := auditCheckByID(t, view, id)
		if check.Enabled {
			t.Fatalf("docker-backed check %q enabled, want disabled", id)
		}
		if strings.TrimSpace(check.DisabledReason) == "" {
			t.Fatalf("docker-backed check %q has no disabled reason, want one", id)
		}
	}

	// The run finished, so the registry holds nothing.
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after run = %+v, want empty (op finished)", got)
	}
	// The banner reads the last result back per project.
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok {
		t.Fatalf("LastAuditResult(project-test) not found, want the completed run")
	}
	if last.SessionID != "sess-1" || last.Status != string(audit.StatusPassed) {
		t.Fatalf("last result = %+v, want sess-1/passed", last)
	}
}

// Progress streams as audit:progress events: running at start, done at end.
func TestAuditService_RunEmitsProgressEvents(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	platform := &desktop.FakePlatform{}
	if _, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project"); err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	var phases []string
	for _, payload := range platform.EventsNamed(desktop.EventAuditProgress) {
		progress, ok := payload.(desktop.AuditProgressPayload)
		if !ok {
			t.Fatalf("audit:progress payload = %#v, want AuditProgressPayload", payload)
		}
		if progress.OpID == "" {
			t.Fatalf("audit:progress payload %+v has empty OpID", progress)
		}
		phases = append(phases, progress.Phase)
	}
	if len(phases) < 2 || phases[0] != "running" || phases[len(phases)-1] != "done" {
		t.Fatalf("audit:progress phases = %v, want [running ... done]", phases)
	}

	// A nil platform must not panic: headless callers get the result anyway.
	stubAuditResolve(t, "project-test")
	if _, err := desktop.RunIntegrityAudit(nil, "/tmp/sample-project"); err != nil {
		t.Fatalf("RunIntegrityAudit(nil platform) error = %v, want nil", err)
	}
}

// Findings are a failed run, not a call error: the view carries status
// failed plus the mapped findings, and the banner sees the failure.
func TestAuditService_FindingsAreFailedResultNotError(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			result := passedIntegrityResult()
			result.Status = audit.StatusFailed
			result.Jobs[0].Status = audit.StatusFailed
			result.Jobs[0].Evidence = map[string]any{
				"findings": []audit.LintFinding{
					{Tool: "composer", Rule: "composer-validate", Path: "composer.json", Line: 3, Message: "license missing"},
				},
			}
			return result, nil
		}))

	view, err := desktop.RunIntegrityAudit(&desktop.FakePlatform{}, "/tmp/sample-project")
	if err != nil {
		t.Fatalf("RunIntegrityAudit(findings) error = %v, want nil (findings are data)", err)
	}
	if view.Status != string(audit.StatusFailed) {
		t.Fatalf("view status = %q, want failed", view.Status)
	}
	if view.FindingCount != 1 || len(view.Findings) != 1 {
		t.Fatalf("view findings = %+v (count=%d), want the 1 reported finding", view.Findings, view.FindingCount)
	}
	finding := view.Findings[0]
	if finding.Tool != "composer" || finding.Path != "composer.json" || finding.Line != 3 || finding.Message != "license missing" {
		t.Fatalf("finding = %+v, want the reported composer finding", finding)
	}
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok || last.Status != string(audit.StatusFailed) {
		t.Fatalf("last result = %+v ok=%v, want the failed run for the banner", last, ok)
	}
}

// Cancelling a long run through the op registry aborts it with a
// cancellation error, cleans the registry, emits a cancelled event, and
// leaves the banner's last completed result untouched.
func TestAuditService_CancelStopsRun(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	started := make(chan struct{})
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			close(started)
			<-ctx.Done()
			return audit.RunResult{}, ctx.Err()
		}))

	type runOutcome struct {
		view desktop.AuditResultView
		err  error
	}
	done := make(chan runOutcome, 1)
	platform := &desktop.FakePlatform{}
	go func() {
		view, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
		done <- runOutcome{view, err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("audit runner never started; RunIntegrityAudit did not reach it")
	}
	var opID string
	for _, op := range desktop.ListOps() {
		if op.Kind == "audit" {
			opID = op.ID
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during run has no audit op: %+v", desktop.ListOps())
	}
	if !desktop.CancelAudit(opID) {
		t.Fatalf("CancelAudit(running run) = false, want true")
	}
	select {
	case outcome := <-done:
		if outcome.err == nil {
			t.Fatalf("cancelled run error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(outcome.err.Error()), "cancel") {
			t.Fatalf("cancelled run error = %v, want a cancellation error", outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled run did not return")
	}
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after cancel = %+v, want empty (CancelOp kills + cleans)", got)
	}
	if _, ok := desktop.LastAuditResult("/tmp/sample-project"); ok {
		t.Fatalf("LastAuditResult after cancel found, want none (cancelled run must not clobber the banner)")
	}
	cancelled := false
	for _, payload := range platform.EventsNamed(desktop.EventAuditProgress) {
		if progress, ok := payload.(desktop.AuditProgressPayload); ok && progress.Phase == "cancelled" {
			cancelled = true
		}
	}
	if !cancelled {
		t.Fatalf("no cancelled audit:progress event emitted")
	}
	// Cancelling a finished or unknown id is a harmless no-op.
	if desktop.CancelAudit(opID) {
		t.Fatalf("CancelAudit(finished op) = true, want false (harmless no-op)")
	}
	if desktop.CancelAudit("audit:no-such-op") {
		t.Fatalf("CancelAudit(unknown op) = true, want false (harmless no-op)")
	}
}

// GetAuditResult reads a persisted run; an empty runID resolves the latest.
func TestAuditService_GetAuditResultReadsPersistedRun(t *testing.T) {
	setupAuditServiceTest(t)
	var gotRunID string
	t.Cleanup(desktop.SetAuditResultReaderForDesktopForTest(
		func(projectID, sessionID, runID string) (audit.RunResult, error) {
			gotRunID = runID
			return passedIntegrityResult(), nil
		}))

	view, err := desktop.GetAuditResult("project-test", "sess-1", "1")
	if err != nil {
		t.Fatalf("GetAuditResult() error = %v", err)
	}
	if gotRunID != "1" {
		t.Fatalf("reader runID = %q, want the explicit run", gotRunID)
	}
	if view.SessionID != "sess-1" || view.Status != string(audit.StatusPassed) {
		t.Fatalf("view = %+v, want sess-1/passed", view)
	}
	integrity := auditCheckByID(t, view, audit.IntegrityCheck)
	if !integrity.Enabled {
		t.Fatalf("integrity check %+v, want enabled", integrity)
	}

	if _, err := desktop.GetAuditResult("", "sess-1", "1"); err == nil {
		t.Fatalf("GetAuditResult(empty project) = nil error, want refusal")
	}
	if _, err := desktop.GetAuditResult("project-test", "", "1"); err == nil {
		t.Fatalf("GetAuditResult(empty session) = nil error, want refusal")
	}
}

// A persisted session without an integrity job renders the integrity row
// disabled with reason instead of claiming a run that never happened.
func TestAuditService_GetAuditResultWithoutIntegrityJobDisablesRow(t *testing.T) {
	setupAuditServiceTest(t)
	t.Cleanup(desktop.SetAuditResultReaderForDesktopForTest(
		func(projectID, sessionID, runID string) (audit.RunResult, error) {
			result := passedIntegrityResult()
			result.Jobs = []audit.JobResult{{ID: "lint", Kind: "container-lint", Status: audit.StatusPassed}}
			return result, nil
		}))

	view, err := desktop.GetAuditResult("project-test", "sess-1", "1")
	if err != nil {
		t.Fatalf("GetAuditResult() error = %v", err)
	}
	integrity := auditCheckByID(t, view, audit.IntegrityCheck)
	if integrity.Enabled {
		t.Fatalf("integrity check %+v, want disabled (session has no integrity job)", integrity)
	}
	if !strings.Contains(strings.ToLower(integrity.DisabledReason), "no integrity run") {
		t.Fatalf("integrity DisabledReason = %q, want a no-run hint", integrity.DisabledReason)
	}
	if len(view.Findings) != 0 || view.FindingCount != 0 {
		t.Fatalf("findings = %+v (count=%d), want none", view.Findings, view.FindingCount)
	}
}

// The desktop's default runner path executes the real container-free
// integrity job (no seam) against the audit store: this proves the service
// wraps the existing integrity paths instead of reimplementing them, and
// that a completed run round-trips through GetAuditResult.
func TestAuditService_DefaultRunnerRunsIntegrityDockerFree(t *testing.T) {
	setupAuditServiceTest(t)
	audit.RegisterIntegrityAnalyzer(desktopTestNoopAnalyzer{})
	projectRoot := t.TempDir()
	t.Cleanup(desktop.SetAuditResolveForDesktopForTest(
		func(projectRoot string) (audit.RunRequest, error) {
			return audit.RunRequest{
				ProjectRoot:      projectRoot,
				ProjectID:        "project-desktop-test",
				Scope:            audit.ScopeProject,
				IntegrityProfile: types.AuditIntegrityProfile{Analyzers: []string{desktopTestNoopAnalyzer{}.ID()}},
				Target:           types.AuditTarget{Framework: "desktop-test", ProjectRoot: projectRoot, TargetPath: projectRoot, Mode: types.AuditTargetStandalone},
			}, nil
		}))
	// No runner seam: the real audit runner executes the real integrity job.

	platform := &desktop.FakePlatform{}
	view, err := desktop.RunIntegrityAudit(platform, projectRoot)
	if err != nil {
		t.Fatalf("RunIntegrityAudit(real runner) error = %v", err)
	}
	if view.Status != string(audit.StatusPassed) {
		t.Fatalf("view status = %q, want passed", view.Status)
	}
	if view.SessionID == "" || view.RunID == "" {
		t.Fatalf("view = %+v, want persisted session/run ids", view)
	}
	integrity := auditCheckByID(t, view, audit.IntegrityCheck)
	if !integrity.Enabled {
		t.Fatalf("integrity check %+v, want enabled", integrity)
	}

	// The run persisted to the real store: GetAuditResult (default reader,
	// JSON round-trip) reads it back.
	stored, err := desktop.GetAuditResult(view.ProjectID, view.SessionID, view.RunID)
	if err != nil {
		t.Fatalf("GetAuditResult(persisted run) error = %v", err)
	}
	if stored.Status != string(audit.StatusPassed) || stored.SessionID != view.SessionID {
		t.Fatalf("stored = %+v, want the persisted passed run", stored)
	}
}

type desktopTestNoopAnalyzer struct{}

func (desktopTestNoopAnalyzer) ID() string { return "desktop-test-noop-integrity" }

func (desktopTestNoopAnalyzer) Analyze(audit.IntegrityRequest) ([]audit.LintFinding, error) {
	return nil, nil
}

// Every user-facing string is sanitized: finding text can carry ANSI escapes
// and control bytes that must never reach the UI.
func TestAuditService_SanitizesUserFacingStrings(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			result := passedIntegrityResult()
			result.Status = audit.StatusFailed
			result.Jobs[0].Status = audit.StatusFailed
			result.Jobs[0].Evidence = map[string]any{
				"findings": []audit.LintFinding{
					{Tool: "\x1b[32mcomposer\x1b[0m", Rule: "rule\x00x", Path: "composer.json\r\n", Message: "bad\rstuff\x07here [1mnow[0m"},
				},
			}
			return result, nil
		}))

	view, err := desktop.RunIntegrityAudit(&desktop.FakePlatform{}, "/tmp/sample-project")
	if err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	if len(view.Findings) != 1 {
		t.Fatalf("findings = %+v, want 1", view.Findings)
	}
	finding := view.Findings[0]
	for _, s := range []string{finding.Tool, finding.Rule, finding.Path, finding.Message} {
		if strings.Contains(s, "\x1b") || strings.Contains(s, "\r") || strings.Contains(s, "\x07") || strings.Contains(s, "\x00") {
			t.Fatalf("unsanitized control bytes in %q", s)
		}
		if strings.Contains(s, "[1m") || strings.Contains(s, "[0m") {
			t.Fatalf("unsanitized ANSI fragment in %q", s)
		}
	}
	if !strings.Contains(finding.Message, "bad") || !strings.Contains(finding.Message, "stuff") {
		t.Fatalf("message over-sanitized: %q", finding.Message)
	}
}

// A framework with no integrity profile is refused before anything runs.
func TestAuditService_FrameworkWithoutIntegrityRefused(t *testing.T) {
	setupAuditServiceTest(t)
	ran := false
	t.Cleanup(desktop.SetAuditResolveForDesktopForTest(
		func(projectRoot string) (audit.RunRequest, error) {
			return audit.RunRequest{}, errors.New(`framework "generic" does not support integrity audit`)
		}))
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			ran = true
			return audit.RunResult{}, nil
		}))

	if _, err := desktop.RunIntegrityAudit(&desktop.FakePlatform{}, "/tmp/sample-project"); err == nil {
		t.Fatalf("RunIntegrityAudit(no integrity profile) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "integrity") {
		t.Fatalf("RunIntegrityAudit(no integrity profile) error = %v, want an integrity hint", err)
	}
	if ran {
		t.Fatalf("runner ran despite refused resolution, want no run")
	}
	if _, ok := desktop.LastAuditResult("/tmp/sample-project"); ok {
		t.Fatalf("LastAuditResult found after refused run, want none")
	}
}

// The last result is persisted to disk: after a simulated restart (memory
// cleared, same GOVARD_HOME) the banner still finds it.
func TestAuditService_LastResultSurvivesRestart(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	if _, err := desktop.RunIntegrityAudit(&desktop.FakePlatform{}, "/tmp/sample-project"); err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	// Simulated restart: in-memory state is gone, the disk copy remains.
	desktop.ResetAuditServiceForTest()
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok {
		t.Fatalf("LastAuditResult after restart not found, want the persisted run")
	}
	if last.SessionID != "sess-1" || last.Status != string(audit.StatusPassed) {
		t.Fatalf("last result = %+v, want sess-1/passed from disk", last)
	}
}

// Two spellings of the same project root resolve to one banner entry.
func TestAuditService_ProjectRootSpellingsShareOneEntry(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	root := t.TempDir()
	if _, err := desktop.RunIntegrityAudit(&desktop.FakePlatform{}, root+string(filepath.Separator)); err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v", err)
	}
	for _, spelling := range []string{root, root + string(filepath.Separator), filepath.Join(root, ".")} {
		last, ok := desktop.LastAuditResult(spelling)
		if !ok {
			t.Fatalf("LastAuditResult(%q) not found, want one entry for every spelling", spelling)
		}
		if last.SessionID != "sess-1" {
			t.Fatalf("LastAuditResult(%q) = %+v, want sess-1", spelling, last)
		}
	}
}

// Cancel then late completion: the completion handler discards the late
// result (never promotes it to the banner) and says so truthfully.
func TestAuditService_CancelThenCompleteDiscardsLateResult(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	calls := 0
	secondStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			calls++
			if calls == 1 {
				return passedIntegrityResult(), nil
			}
			secondStarted <- struct{}{}
			<-release
			if ctx.Err() != nil {
				return audit.RunResult{}, ctx.Err()
			}
			late := passedIntegrityResult()
			late.SessionID = "sess-2"
			return late, nil
		}))

	platform := &desktop.FakePlatform{}
	if _, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project"); err != nil {
		t.Fatalf("first RunIntegrityAudit() error = %v", err)
	}

	type runOutcome struct {
		view desktop.AuditResultView
		err  error
	}
	done := make(chan runOutcome, 1)
	go func() {
		view, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
		done <- runOutcome{view, err}
	}()

	select {
	case <-secondStarted:
	case <-time.After(5 * time.Second):
		t.Fatalf("second audit run never started")
	}
	var opID string
	for _, op := range desktop.ListOps() {
		if op.Kind == "audit" {
			opID = op.ID
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during run has no audit op: %+v", desktop.ListOps())
	}
	if !desktop.CancelAudit(opID) {
		t.Fatalf("CancelAudit(running run) = false, want true")
	}
	close(release)
	select {
	case outcome := <-done:
		if outcome.err == nil {
			t.Fatalf("cancelled run error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(outcome.err.Error()), "discard") {
			t.Fatalf("cancelled run error = %v, want truthful discard wording", outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled run did not return")
	}
	// The late sess-2 completion was discarded: the banner keeps sess-1.
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok {
		t.Fatalf("LastAuditResult missing after discard, want the prior completed run")
	}
	if last.SessionID != "sess-1" {
		t.Fatalf("last result = %+v, want sess-1 (late sess-2 must be discarded)", last)
	}
}

func findingForDiff(tool, rule, path string, line int, message string) audit.LintFinding {
	return audit.LintFinding{Tool: tool, Rule: rule, Path: path, Line: line, Message: message}
}

func diffResultWith(runID string, findings []audit.LintFinding) audit.RunResult {
	result := passedIntegrityResult()
	result.RunID = runID
	result.Status = audit.StatusFailed
	result.Jobs[0].Status = audit.StatusFailed
	result.Jobs[0].Evidence = map[string]any{"findings": findings}
	return result
}

// GetAuditDiff summarizes two stored runs: added, removed, unchanged.
func TestAuditService_GetAuditDiffSummarizesRuns(t *testing.T) {
	setupAuditServiceTest(t)
	t.Cleanup(desktop.SetAuditRunListForDesktopForTest(
		func(projectID, sessionID string) ([]string, error) {
			return []string{"1", "2"}, nil
		}))
	t.Cleanup(desktop.SetAuditResultReaderForDesktopForTest(
		func(projectID, sessionID, runID string) (audit.RunResult, error) {
			shared := findingForDiff("composer", "rule-b", "composer.json", 2, "stays")
			if runID == "1" {
				return diffResultWith("1", []audit.LintFinding{
					findingForDiff("composer", "rule-a", "composer.json", 1, "gone"),
					shared,
				}), nil
			}
			return diffResultWith("2", []audit.LintFinding{
				shared,
				findingForDiff("composer", "rule-c", "composer.json", 3, "new"),
			}), nil
		}))

	diff, err := desktop.GetAuditDiff("project-test", "sess-1", "", "")
	if err != nil {
		t.Fatalf("GetAuditDiff() error = %v", err)
	}
	if diff.FromRunID != "1" || diff.ToRunID != "2" {
		t.Fatalf("diff range = %s..%s, want 1..2 (latest plus previous)", diff.FromRunID, diff.ToRunID)
	}
	if len(diff.Added) != 1 || diff.Added[0].Rule != "rule-c" {
		t.Fatalf("added = %+v, want rule-c", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Rule != "rule-a" {
		t.Fatalf("removed = %+v, want rule-a", diff.Removed)
	}
	if diff.UnchangedCount != 1 {
		t.Fatalf("unchanged = %d, want 1 (rule-b)", diff.UnchangedCount)
	}
}

// Missing sessions and single-run sessions refuse a diff with an error.
func TestAuditService_GetAuditDiffMissingSessionErrors(t *testing.T) {
	setupAuditServiceTest(t)
	t.Cleanup(desktop.SetAuditResultReaderForDesktopForTest(
		func(projectID, sessionID, runID string) (audit.RunResult, error) {
			return audit.RunResult{}, errors.New(`audit session "nope" not found`)
		}))

	if _, err := desktop.GetAuditDiff("project-test", "nope", "1", "2"); err == nil {
		t.Fatalf("GetAuditDiff(missing session) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "diff") {
		t.Fatalf("GetAuditDiff(missing session) error = %v, want a diff error", err)
	}

	t.Cleanup(desktop.SetAuditRunListForDesktopForTest(
		func(projectID, sessionID string) ([]string, error) {
			return []string{"1"}, nil
		}))
	if _, err := desktop.GetAuditDiff("project-test", "sess-1", "", ""); err == nil {
		t.Fatalf("GetAuditDiff(single run, no from) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "earlier") {
		t.Fatalf("GetAuditDiff(single run, no from) error = %v, want a no-earlier-run hint", err)
	}

	if _, err := desktop.GetAuditDiff("", "sess-1", "1", "2"); err == nil {
		t.Fatalf("GetAuditDiff(empty project) = nil error, want refusal")
	}
}

// A cancel landing between the completion check and the banner record must
// still discard the late result: the check and the record share one lock, so
// the pre-record hook delivers CancelAudit deterministically in that window.
func TestAuditService_CancelBetweenCheckAndRecordDiscards(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	calls := 0
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			calls++
			result := passedIntegrityResult()
			if calls > 1 {
				result.SessionID = "sess-2"
			}
			return result, nil
		}))
	// A hook restore of nil keeps the other tests on the production path.
	t.Cleanup(desktop.SetAuditPreRecordHookForDesktopForTest(nil))

	platform := &desktop.FakePlatform{}
	if _, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project"); err != nil {
		t.Fatalf("first RunIntegrityAudit() error = %v", err)
	}

	t.Cleanup(desktop.SetAuditPreRecordHookForDesktopForTest(func(opID string) {
		if !desktop.CancelAudit(opID) {
			t.Errorf("CancelAudit(%s) in pre-record hook = false, want true (op still registered)", opID)
		}
	}))
	_, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
	if err == nil {
		t.Fatalf("run cancelled in check-record window error = nil, want discard")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "discard") {
		t.Fatalf("run cancelled in check-record window error = %v, want discard wording", err)
	}
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok {
		t.Fatalf("LastAuditResult missing after window cancel, want the prior completed run")
	}
	if last.SessionID != "sess-1" {
		t.Fatalf("last result = %+v, want sess-1 (late sess-2 must be discarded)", last)
	}
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after window cancel = %+v, want empty", got)
	}
}

// A completed run that cannot be recorded (here: no usable working directory
// to key it by) fails neutrally: no cancelled event, no cancel wording, no
// banner entry.
func TestAuditService_UnrecordableResultFailsNeutrally(t *testing.T) {
	setupAuditServiceTest(t)
	t.Cleanup(desktop.SetAuditResolveForDesktopForTest(
		func(projectRoot string) (audit.RunRequest, error) {
			return audit.RunRequest{
				ProjectRoot: "",
				ProjectID:   "project-test",
				Scope:       audit.ScopeProject,
			}, nil
		}))
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	// Break getwd so the empty root normalizes to nothing: the result has no
	// key and must not record. t.Chdir restores the directory on cleanup.
	nowhere := t.TempDir()
	t.Chdir(nowhere)
	if err := os.Remove(nowhere); err != nil {
		t.Fatalf("remove cwd %s: %v", nowhere, err)
	}

	platform := &desktop.FakePlatform{}
	_, err := desktop.RunIntegrityAudit(platform, "")
	if err == nil {
		t.Fatalf("RunIntegrityAudit(unkeyable root) error = nil, want neutral failure")
	}
	lowered := strings.ToLower(err.Error())
	if strings.Contains(lowered, "cancel") || strings.Contains(lowered, "discard") {
		t.Fatalf("RunIntegrityAudit(unkeyable root) error = %v, want neutral wording (not cancelled)", err)
	}
	for _, payload := range platform.EventsNamed(desktop.EventAuditProgress) {
		progress, ok := payload.(desktop.AuditProgressPayload)
		if !ok {
			continue
		}
		if progress.Phase == "cancelled" {
			t.Fatalf("emitted a cancelled audit:progress event for a non-cancelled failure: %+v", progress)
		}
	}
	if _, ok := desktop.LastAuditResult(""); ok {
		t.Fatalf("LastAuditResult recorded an unkeyable run, want none")
	}
	if got := desktop.AuditCancelledSetSizeForTest(); got != 0 {
		t.Fatalf("cancelled-set size after empty-key refusal = %d, want 0 (no mark)", got)
	}
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after neutral failure = %+v, want empty", got)
	}
}

// A cancel acknowledged strictly after the record (too late to discard, so
// correctly promoted) must still leave no mark: the post-FinishOp scrub
// removes it.
func TestAuditService_PostRecordAckLeavesNoMark(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			return passedIntegrityResult(), nil
		}))

	platform := &cancelOnDoneAuditPlatform{FakePlatform: &desktop.FakePlatform{}}
	view, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
	if err != nil {
		t.Fatalf("RunIntegrityAudit() error = %v, want nil (cancel arrived post-record)", err)
	}
	if view.SessionID != "sess-1" {
		t.Fatalf("view = %+v, want the recorded sess-1 run", view)
	}
	if !platform.cancelled {
		t.Fatalf("done-hook cancel did not run, want a true post-record ack")
	}
	if got := desktop.AuditCancelledSetSizeForTest(); got != 0 {
		t.Fatalf("cancelled-set size = %d, want 0 (post-record ack scrubbed)", got)
	}
	last, ok := desktop.LastAuditResult("/tmp/sample-project")
	if !ok || last.SessionID != "sess-1" {
		t.Fatalf("last result = %+v ok=%v, want the recorded sess-1 run", last, ok)
	}
}

// cancelOnDoneAuditPlatform cancels the run exactly when its done event
// arrives — i.e. after the record, before deferred FinishOp — reproducing the
// post-record-ack residue scenario deterministically.
type cancelOnDoneAuditPlatform struct {
	*desktop.FakePlatform
	cancelled bool
}

func (p *cancelOnDoneAuditPlatform) Emit(event string, data any) {
	p.FakePlatform.Emit(event, data)
	if event != desktop.EventAuditProgress {
		return
	}
	progress, ok := data.(desktop.AuditProgressPayload)
	if !ok || progress.Phase != "done" {
		return
	}
	if desktop.CancelAudit(progress.OpID) {
		p.cancelled = true
	}
}

// The age-purge bounds the set: many stale unacknowledged inserts collapse to
// nothing on the next insert.
func TestAuditService_CancelledSetPurgeBoundsStaleMarks(t *testing.T) {
	setupAuditServiceTest(t)
	desktop.PlantAuditCancelledMarksForTest(200, 0, 2*time.Hour)
	if got := desktop.AuditCancelledSetSizeForTest(); got != 200 {
		t.Fatalf("cancelled-set size after planting = %d, want 200", got)
	}
	if desktop.CancelAudit("audit:no-such-op") {
		t.Fatalf("CancelAudit(unknown) = true, want false")
	}
	if got := desktop.AuditCancelledSetSizeForTest(); got != 0 {
		t.Fatalf("cancelled-set size after purge trigger = %d, want 0", got)
	}
}

// A stale won mark survives the purge and still discards with a live
// context: the record consults only the set, so a wrongly-purged won mark
// would promote here.
func TestAuditService_WonMarkSurvivesPurgeAndDiscards(t *testing.T) {
	setupAuditServiceTest(t)
	stubAuditResolve(t, "project-test")
	t.Cleanup(desktop.SetAuditRunnerForDesktopForTest(
		func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error) {
			var opID string
			for _, op := range desktop.ListOps() {
				if op.Kind == "audit" {
					opID = op.ID
				}
			}
			if opID == "" {
				t.Errorf("ListOps during run has no audit op")
				return passedIntegrityResult(), nil
			}
			desktop.PlantAuditCancelledMarkForOpForTest(opID, true, 2*time.Hour)
			desktop.PlantAuditCancelledMarksForTest(200, 0, 2*time.Hour)
			desktop.CancelAudit("audit:no-such-op")
			if got := desktop.AuditCancelledSetSizeForTest(); got != 1 {
				t.Errorf("cancelled-set size after purge = %d, want 1 (the won mark)", got)
			}
			return passedIntegrityResult(), nil
		}))

	platform := &desktop.FakePlatform{}
	_, err := desktop.RunIntegrityAudit(platform, "/tmp/sample-project")
	if err == nil {
		t.Fatalf("run with surviving won mark error = nil, want discard")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "discard") {
		t.Fatalf("run with surviving won mark error = %v, want discard wording", err)
	}
	if _, ok := desktop.LastAuditResult("/tmp/sample-project"); ok {
		t.Fatalf("LastAuditResult found after won-mark discard, want none")
	}
	if got := desktop.AuditCancelledSetSizeForTest(); got != 0 {
		t.Fatalf("cancelled-set size after discard = %d, want 0", got)
	}
}
