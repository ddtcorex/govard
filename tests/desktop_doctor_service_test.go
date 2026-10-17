package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// stubDoctorServiceReport covers every fix kind: one safe check, one
// risky check, one check with no fix, and one passing check.
func stubDoctorServiceReport() engine.DoctorReport {
	return engine.DoctorReport{
		Checks: []engine.DoctorCheck{
			{ID: "host.govard.home", Title: "Govard home directory", Status: engine.DoctorStatusWarn, Message: "home not ready"},
			{ID: "project.config.drift", Title: "Configuration drift", Status: engine.DoctorStatusWarn, Message: "drifted"},
			{ID: "host.port.80", Title: "Host port 80", Status: engine.DoctorStatusWarn, Message: "port in use"},
			{ID: "docker.daemon", Title: "Docker daemon", Status: engine.DoctorStatusPass, Message: "running"},
		},
	}
}

func setupDoctorServiceTest(t *testing.T) {
	t.Helper()
	desktop.ResetDoctorServiceForTest()
	desktop.ResetOperationRegistryForTest()
	t.Cleanup(desktop.ResetDoctorServiceForTest)
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

func stubDoctorDiagnostics(t *testing.T) {
	t.Helper()
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(stubDoctorServiceReport))
}

func doctorFixKinds(t *testing.T, report desktop.DoctorReportView) map[string]desktop.DoctorFixKind {
	t.Helper()
	kinds := map[string]desktop.DoctorFixKind{}
	for _, check := range report.Checks {
		kinds[check.ID] = check.FixKind
	}
	return kinds
}

// A doctor run wraps the engine and classifies every check into the fix
// allowlist: safe (1-click), risky (preview+confirm), none (no fix path).
func TestDoctorService_RunClassifiesFixKinds(t *testing.T) {
	setupDoctorServiceTest(t)
	stubDoctorDiagnostics(t)

	report, err := desktop.RunDoctor()
	if err != nil {
		t.Fatalf("RunDoctor() error = %v", err)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("RunDoctor() returned %d checks, want 4", len(report.Checks))
	}
	wantKinds := map[string]desktop.DoctorFixKind{
		"host.govard.home":     desktop.DoctorFixKindSafe,
		"project.config.drift": desktop.DoctorFixKindRisky,
		"host.port.80":         desktop.DoctorFixKindNone,
		"docker.daemon":        desktop.DoctorFixKindNone,
	}
	if got := doctorFixKinds(t, report); !equalFixKinds(got, wantKinds) {
		t.Fatalf("fix kinds = %v, want %v", got, wantKinds)
	}
	for _, check := range report.Checks {
		switch check.FixKind {
		case desktop.DoctorFixKindSafe:
			if check.FixLabel == "" || check.NeedsPreview {
				t.Fatalf("safe check %s: FixLabel=%q NeedsPreview=%v, want label and no preview", check.ID, check.FixLabel, check.NeedsPreview)
			}
		case desktop.DoctorFixKindRisky:
			if check.FixLabel == "" || !check.NeedsPreview {
				t.Fatalf("risky check %s: FixLabel=%q NeedsPreview=%v, want label and preview", check.ID, check.FixLabel, check.NeedsPreview)
			}
		case desktop.DoctorFixKindNone:
			if check.FixLabel != "" || check.NeedsPreview {
				t.Fatalf("none check %s: FixLabel=%q NeedsPreview=%v, want no fix path at all", check.ID, check.FixLabel, check.NeedsPreview)
			}
		}
	}
	if report.Passed != 1 || report.Warnings != 3 || report.Failures != 0 {
		t.Fatalf("counts = passed=%d warnings=%d failures=%d, want 1/3/0", report.Passed, report.Warnings, report.Failures)
	}
	if report.OpID == "" {
		t.Fatalf("RunDoctor() returned empty OpID, want a registry op id")
	}
	// The run finished, so the registry holds nothing and a late cancel is
	// a harmless no-op (same contract as CancelOp after FinishOp).
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after run = %+v, want empty (op finished)", got)
	}
	if desktop.CancelDoctorRun(report.OpID) {
		t.Fatalf("CancelDoctorRun(finished op) = true, want false (harmless no-op)")
	}
}

func equalFixKinds(a, b map[string]desktop.DoctorFixKind) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range b {
		if a[k] != v {
			return false
		}
	}
	return true
}

type doctorExecutorCall struct {
	checkID string
	dryRun  bool
}

// A safe fix applies directly: no preview token, no confirm flag.
func TestDoctorService_SafeFixAppliesDirectly(t *testing.T) {
	setupDoctorServiceTest(t)
	var calls []doctorExecutorCall
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			calls = append(calls, doctorExecutorCall{checkID, dryRun})
			return []string{"mkdir -p govard home"}, "home repaired", nil
		}))

	outcome, err := desktop.ApplyDoctorFix("host.govard.home", "", false)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(safe, no token, no confirm) error = %v", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied || outcome.CheckID != "host.govard.home" {
		t.Fatalf("outcome = %+v, want applied for host.govard.home", outcome)
	}
	if len(calls) != 1 || calls[0].checkID != "host.govard.home" || calls[0].dryRun {
		t.Fatalf("executor calls = %+v, want one real (dryRun=false) call", calls)
	}

	// Safe stays 1-click even when the caller passes junk token/confirm.
	if _, err := desktop.ApplyDoctorFix("host.govard.home", "bogus-token", false); err != nil {
		t.Fatalf("ApplyDoctorFix(safe, bogus token) error = %v, want nil (token ignored)", err)
	}

	// Safe needs no preview: asking for one is refused (with a
	// direct-apply hint), and surrounding whitespace is trimmed.
	if _, err := desktop.PreviewDoctorFix("host.govard.home"); err == nil {
		t.Fatalf("PreviewDoctorFix(safe) = nil error, want refusal (applies directly)")
	} else if !strings.Contains(strings.ToLower(err.Error()), "directly") {
		t.Fatalf("PreviewDoctorFix(safe) error = %v, want a direct-apply hint", err)
	}
	if _, err := desktop.ApplyDoctorFix("  host.govard.home  ", "", false); err != nil {
		t.Fatalf("ApplyDoctorFix(padded safe id) error = %v, want nil (id trimmed)", err)
	}
}

// A risky fix without its preview token is refused before anything runs.
func TestDoctorService_RiskyFixRefusedWithoutPreviewToken(t *testing.T) {
	setupDoctorServiceTest(t)
	var calls []doctorExecutorCall
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			calls = append(calls, doctorExecutorCall{checkID, dryRun})
			return []string{"mutated"}, "mutated", nil
		}))

	for _, tc := range []struct {
		name      string
		previewID string
		confirm   bool
	}{
		{"no token no confirm", "", false},
		{"confirm without token", "", true},
		{"bogus token with confirm", "bogus-token", true},
	} {
		if _, err := desktop.ApplyDoctorFix("project.config.drift", tc.previewID, tc.confirm); err == nil {
			t.Fatalf("ApplyDoctorFix(risky, %s) = nil error, want refusal", tc.name)
		} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
			t.Fatalf("ApplyDoctorFix(risky, %s) error = %v, want a preview-token hint", tc.name, err)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("executor calls = %+v, want none (refused before execute)", calls)
	}
}

// A risky fix runs only after a dry-run preview minted its token, with an
// explicit confirm; the token is single-use and bound to its check.
func TestDoctorService_RiskyFixPreviewThenApplyWithConfirm(t *testing.T) {
	setupDoctorServiceTest(t)
	var calls []doctorExecutorCall
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			calls = append(calls, doctorExecutorCall{checkID, dryRun})
			if dryRun {
				return []string{"sync php_version", "sync db_version"}, "2 drift entries", nil
			}
			return []string{"synced drift"}, "drift synced", nil
		}))

	preview, err := desktop.PreviewDoctorFix("project.config.drift")
	if err != nil {
		t.Fatalf("PreviewDoctorFix(risky) error = %v", err)
	}
	if preview.PreviewID == "" || preview.CheckID != "project.config.drift" || !preview.RequiresConfirm {
		t.Fatalf("preview = %+v, want token for project.config.drift requiring confirm", preview)
	}
	if len(preview.Actions) == 0 {
		t.Fatalf("preview actions empty, want the dry-run plan")
	}
	// Preview is a dry run: it plans but never mutates.
	for _, call := range calls {
		if !call.dryRun {
			t.Fatalf("executor calls = %+v, want only dry-run calls during preview", calls)
		}
	}
	planned := len(calls)

	// The token alone is not enough: confirm is required too.
	if _, err := desktop.ApplyDoctorFix("project.config.drift", preview.PreviewID, false); err == nil {
		t.Fatalf("ApplyDoctorFix(risky, token, confirm=false) = nil error, want refusal")
	}

	outcome, err := desktop.ApplyDoctorFix("project.config.drift", preview.PreviewID, true)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(risky, token, confirm=true) error = %v", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied {
		t.Fatalf("outcome = %+v, want applied", outcome)
	}
	if len(calls) != planned+2 || !calls[len(calls)-2].dryRun || calls[len(calls)-1].dryRun {
		t.Fatalf("executor calls = %+v, want a dry-run re-verify then one real apply", calls)
	}

	// Single-use: replaying the consumed token is refused.
	if _, err := desktop.ApplyDoctorFix("project.config.drift", preview.PreviewID, true); err == nil {
		t.Fatalf("ApplyDoctorFix(replayed token) = nil error, want refusal")
	}
}

// A preview token minted for one check never authorizes another.
func TestDoctorService_PreviewTokenBoundToCheck(t *testing.T) {
	setupDoctorServiceTest(t)
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			return []string{"plan"}, "plan", nil
		}))

	preview, err := desktop.PreviewDoctorFix("project.config.drift")
	if err != nil {
		t.Fatalf("PreviewDoctorFix error = %v", err)
	}
	if _, err := desktop.ApplyDoctorFix("project.config.audit", preview.PreviewID, true); err == nil {
		t.Fatalf("ApplyDoctorFix(other check, drift token) = nil error, want refusal")
	}
}

// Checks with no safe fix expose no fix path: neither preview nor apply.
func TestDoctorService_NoneCheckHasNoFixPath(t *testing.T) {
	setupDoctorServiceTest(t)
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			t.Errorf("executor must not run for fixKind none (check %s)", checkID)
			return nil, "", nil
		}))

	if _, err := desktop.PreviewDoctorFix("host.port.80"); err == nil {
		t.Fatalf("PreviewDoctorFix(none) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "no fix") {
		t.Fatalf("PreviewDoctorFix(none) error = %v, want a no-fix hint", err)
	}
	if _, err := desktop.ApplyDoctorFix("host.port.80", "", true); err == nil {
		t.Fatalf("ApplyDoctorFix(none) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "no fix") {
		t.Fatalf("ApplyDoctorFix(none) error = %v, want a no-fix hint", err)
	}
	if _, err := desktop.PreviewDoctorFix("no.such.check"); err == nil {
		t.Fatalf("PreviewDoctorFix(unknown id) = nil error, want refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("PreviewDoctorFix(unknown id) error = %v, want an unknown-check hint", err)
	}
}

// Every user-facing string is sanitized: engine text can carry ANSI
// escapes and control bytes that must never reach the UI.
func TestDoctorService_SanitizesUserFacingStrings(t *testing.T) {
	setupDoctorServiceTest(t)
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		return engine.DoctorReport{Checks: []engine.DoctorCheck{
			{ID: "host.govard.home", Title: "\x1b[32mHome\x1b[0m", Status: engine.DoctorStatusWarn,
				Message: "bad\rstuff\x07here", Hint: "[1mhint[0m", SuggestedCommand: "govard\x00 doctor"},
		}}
	}))
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			return []string{"\x1b[33mdid x\x1b[0m"}, "ok\x00done", nil
		}))

	report, err := desktop.RunDoctor()
	if err != nil {
		t.Fatalf("RunDoctor() error = %v", err)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(report.Checks))
	}
	view := report.Checks[0]
	for _, s := range []string{view.Title, view.Message, view.Hint, view.SuggestedCommand} {
		if strings.Contains(s, "\x1b") || strings.Contains(s, "\r") || strings.Contains(s, "\x07") || strings.Contains(s, "\x00") {
			t.Fatalf("unsanitized control bytes in %q", s)
		}
		if strings.Contains(s, "[1m") || strings.Contains(s, "[0m") {
			t.Fatalf("unsanitized ANSI fragment in %q", s)
		}
	}
	if !strings.Contains(view.Message, "bad") || !strings.Contains(view.Message, "stuff") {
		t.Fatalf("message over-sanitized: %q", view.Message)
	}

	outcome, err := desktop.ApplyDoctorFix("host.govard.home", "", false)
	if err != nil {
		t.Fatalf("ApplyDoctorFix error = %v", err)
	}
	if strings.Contains(outcome.Message, "\x00") || strings.Contains(outcome.Actions[0], "\x1b") {
		t.Fatalf("unsanitized fix outcome: %+v", outcome)
	}
}

// Cancelling a long doctor run mid-flight aborts it through the op registry.
func TestDoctorService_CancelDuringRunAbortsLongRun(t *testing.T) {
	setupDoctorServiceTest(t)
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	t.Cleanup(desktop.SetDoctorRunDiagnosticsForDesktopForTest(func() engine.DoctorReport {
		close(started)
		<-release
		return stubDoctorServiceReport()
	}))

	type runResult struct {
		report desktop.DoctorReportView
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		report, err := desktop.RunDoctor()
		done <- runResult{report, err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("doctor run never reached the engine; RunDoctor did not start it")
	}
	var opID string
	for _, op := range desktop.ListOps() {
		if op.Kind == "doctor" {
			opID = op.ID
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during run has no doctor op: %+v", desktop.ListOps())
	}
	if !desktop.CancelDoctorRun(opID) {
		t.Fatalf("CancelDoctorRun(running run) = false, want true")
	}
	select {
	case res := <-done:
		if res.err == nil {
			t.Fatalf("cancelled run error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(res.err.Error()), "cancel") {
			t.Fatalf("cancelled run error = %v, want a cancellation error", res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled run did not return")
	}
}

// Cancelling a long fix mid-flight aborts it through the op registry.
func TestDoctorService_CancelDuringApplyAbortsLongFix(t *testing.T) {
	setupDoctorServiceTest(t)
	started := make(chan struct{})
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			close(started)
			<-ctx.Done()
			return nil, "", ctx.Err()
		}))

	type applyResult struct {
		outcome desktop.DoctorFixOutcome
		err     error
	}
	done := make(chan applyResult, 1)
	go func() {
		outcome, err := desktop.ApplyDoctorFix("host.govard.home", "", false)
		done <- applyResult{outcome, err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("fix executor never started; ApplyDoctorFix did not reach the executor")
	}
	var opID string
	for _, op := range desktop.ListOps() {
		if op.Kind == "doctor-fix" {
			opID = op.ID
		}
	}
	if opID == "" {
		t.Fatalf("ListOps during apply has no doctor-fix op: %+v", desktop.ListOps())
	}
	if !desktop.CancelDoctorRun(opID) {
		t.Fatalf("CancelDoctorRun(running fix) = false, want true")
	}
	select {
	case res := <-done:
		if res.err == nil {
			t.Fatalf("cancelled apply error = nil, want cancellation")
		}
		if !strings.Contains(strings.ToLower(res.err.Error()), "cancel") {
			t.Fatalf("cancelled apply error = %v, want a cancellation error", res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelled apply did not return")
	}
	if got := desktop.ListOps(); len(got) != 0 {
		t.Fatalf("ListOps after cancelled apply = %+v, want empty", got)
	}
}

// The preview token binds the plan hash, not just the check: state that
// drifts between preview and apply is refused before anything mutates.
func TestDoctorService_RiskyFixRefusedWhenStateDriftsAfterPreview(t *testing.T) {
	setupDoctorServiceTest(t)
	var realCalls []doctorExecutorCall
	dryRuns := 0
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			if dryRun {
				dryRuns++
				if dryRuns == 1 {
					return []string{"sync php_version"}, "1 drift entry", nil
				}
				return []string{"sync php_version", "sync db_version"}, "2 drift entries", nil
			}
			realCalls = append(realCalls, doctorExecutorCall{checkID, dryRun})
			return []string{"synced"}, "synced", nil
		}))

	preview, err := desktop.PreviewDoctorFix("project.config.drift")
	if err != nil {
		t.Fatalf("PreviewDoctorFix error = %v", err)
	}
	if _, err := desktop.ApplyDoctorFix("project.config.drift", preview.PreviewID, true); err == nil {
		t.Fatalf("ApplyDoctorFix(drifted plan) = nil error, want re-preview refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "re-preview") {
		t.Fatalf("ApplyDoctorFix(drifted plan) error = %v, want a re-preview hint", err)
	}
	if len(realCalls) != 0 {
		t.Fatalf("real executor calls = %+v, want none (refused before execute)", realCalls)
	}
}

// The apply re-verifies the plan: identical state between preview and apply
// goes through, proving the hash comparison is equality, not blanket refusal.
func TestDoctorService_RiskyFixAppliesWhenStateUnchanged(t *testing.T) {
	setupDoctorServiceTest(t)
	dryRuns := 0
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			if dryRun {
				dryRuns++
				return []string{"sync php_version"}, "1 drift entry", nil
			}
			return []string{"synced"}, "synced", nil
		}))

	preview, err := desktop.PreviewDoctorFix("project.config.drift")
	if err != nil {
		t.Fatalf("PreviewDoctorFix error = %v", err)
	}
	outcome, err := desktop.ApplyDoctorFix("project.config.drift", preview.PreviewID, true)
	if err != nil {
		t.Fatalf("ApplyDoctorFix(unchanged plan) error = %v", err)
	}
	if outcome.Status != desktop.DoctorFixOutcomeApplied {
		t.Fatalf("outcome = %+v, want applied", outcome)
	}
	if dryRuns != 2 {
		t.Fatalf("dry-run plans computed = %d, want 2 (preview plus apply-time re-verify)", dryRuns)
	}
}

// A directory at the registry file path means corruption for human eyes:
// the fix refuses and deletes nothing.
func TestDoctorService_RegistryDirRefusesWithoutDeleting(t *testing.T) {
	setupDoctorServiceTest(t)
	registryDir := filepath.Join(t.TempDir(), "registry-dir")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(registryDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", registryDir)
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(nil))

	if _, err := desktop.ApplyDoctorFix("host.govard.registry", "", false); err == nil {
		t.Fatalf("ApplyDoctorFix(registry dir) = nil error, want refusal (never auto-delete)")
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel missing after refused fix: %v (contents must stay intact)", err)
	}
	if info, err := os.Stat(registryDir); err != nil || !info.IsDir() {
		t.Fatalf("registry dir missing after refused fix (stat=%v info=%v)", err, info)
	}
}

// Token minting without randomness must fail with NO token: an unusable
// token that Apply can never honor is worse than an error.
func TestDoctorService_PreviewRandFailureReturnsNoToken(t *testing.T) {
	setupDoctorServiceTest(t)
	t.Cleanup(desktop.SetDoctorFixExecutorForDesktopForTest(
		func(ctx context.Context, checkID string, dryRun bool) ([]string, string, error) {
			return []string{"plan"}, "plan", nil
		}))
	t.Cleanup(desktop.SetDoctorRandReadForDesktopForTest(
		func([]byte) (int, error) { return 0, errors.New("no entropy") }))

	preview, err := desktop.PreviewDoctorFix("project.config.drift")
	if err == nil {
		t.Fatalf("PreviewDoctorFix(rand failure) = nil error, want mint failure")
	}
	if preview.PreviewID != "" {
		t.Fatalf("PreviewDoctorFix(rand failure) PreviewID = %q, want empty (no unusable token)", preview.PreviewID)
	}
}
