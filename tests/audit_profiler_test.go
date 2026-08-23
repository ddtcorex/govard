package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
)

func TestAuditProfilerStoresCollectedCSVAndRestoresLease(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{csv: []byte("type,timer\nfoo,1\n")}
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:           store,
		ProfilerRuntime: runtime,
		Resources:       audit.Resources{CPU: 2, MemoryMB: 2048},
	})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != audit.StatusPassed {
		t.Fatalf("status = %q, want passed", result.Status)
	}
	if want := []string{"activate", "capture", "collect", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one profiler CSV", result.Artifacts)
	}
	artifact := result.Artifacts[0]
	if artifact.Kind != "profiler-csv" {
		t.Fatalf("artifact kind = %q, want profiler-csv", artifact.Kind)
	}
	if artifact.SHA256 != "2443521cb316804b3588c092e1d96f5e7ea6fb8a0147111e257591831694d909" {
		t.Fatalf("artifact SHA256 = %q", artifact.SHA256)
	}
	if want := filepath.Join("artifacts", "profiler", "profile.csv"); filepath.Base(filepath.Dir(filepath.Dir(artifact.Path))) != "artifacts" || filepath.Base(filepath.Dir(artifact.Path)) != "profiler" || filepath.Base(artifact.Path) != "profile.csv" {
		t.Fatalf("artifact path = %q, want run-isolated %q", artifact.Path, want)
	}
	got, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "type,timer\nfoo,1\n" {
		t.Fatalf("artifact contents = %q", got)
	}
	persisted, err := store.ReadResult(result.ProjectID, result.SessionID, result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.Artifacts, result.Artifacts) {
		t.Fatalf("persisted artifacts = %#v, want %#v", persisted.Artifacts, result.Artifacts)
	}
	if _, err := store.AcquireLease(result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("profiler lease was not released: %v", err)
	}
}

func TestAuditProfilerRerunPreservesExactURL(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{csv: []byte("type,timer\nfoo,1\n")}
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:           store,
		ProfilerRuntime: runtime,
		Resources:       audit.Resources{CPU: 2, MemoryMB: 2048},
	})
	request := profilerRunRequest()
	request.ProfilerURL = "https://shop.test/catalogsearch/result/?q=red%20shoe&product_list_limit=48"

	first, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Rerun(context.Background(), first.SessionID, first.ProjectID, []string{"profiler"}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.activateRequests) != 2 {
		t.Fatalf("activate requests = %d, want 2", len(runtime.activateRequests))
	}
	for index, got := range runtime.activateRequests {
		if got.URL != request.ProfilerURL {
			t.Fatalf("activate request %d URL = %q, want %q", index, got.URL, request.ProfilerURL)
		}
	}
}

func TestAuditProfilerDoesNotActivateWhileDiagnosticsLeaseIsHeld(t *testing.T) {
	store := newDeterministicAuditStore(t)
	request := profilerRunRequest()
	if _, err := store.AcquireLease(request.ProjectID, "diagnostics", "other-run"); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeProfilerRuntime{}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), request)
	if err == nil {
		t.Fatal("run succeeded while diagnostics lease was held")
	}
	if result.Status != audit.StatusFailed || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want a persisted infrastructure failure", result)
	}
	if len(runtime.events) != 0 {
		t.Fatalf("runtime events = %#v, want no activation before lease acquisition", runtime.events)
	}
}

func TestAuditProfilerRestoresAndReleasesAfterActivationFailure(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{activateErr: errors.New("activate profiler")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err == nil || err.Error() != "activate profiler" {
		t.Fatalf("run error = %v, want activation failure", err)
	}
	if result.Status != audit.StatusFailed || len(result.Artifacts) != 0 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want failed result without artifacts", result)
	}
	if want := []string{"activate", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if _, err := store.AcquireLease(result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("profiler lease was not released: %v", err)
	}
}

func TestAuditProfilerPreservesOperationAndRestoreFailuresInTerminalEvidence(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{
		activateErr: errors.New("activate profiler"),
		restoreErr:  errors.New("restore profiler"),
	}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err == nil || !strings.Contains(err.Error(), "activate profiler") || !strings.Contains(err.Error(), "restore profiler") {
		t.Fatalf("run error = %v, want operation and restore failures", err)
	}
	if result.Status != audit.StatusFailed || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want failed result with one infrastructure error", result)
	}
	message := result.Errors[0].Message
	if !strings.Contains(message, "activate profiler") || !strings.Contains(message, "restore profiler") {
		t.Fatalf("terminal error = %q, want operation and restore failures", message)
	}
	if want := []string{"activate", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
}

func TestAuditProfilerRestoresAndReleasesAfterCaptureFailure(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{captureErr: errors.New("capture profiler")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err == nil || err.Error() != "capture profiler" {
		t.Fatalf("run error = %v, want capture failure", err)
	}
	if result.Status != audit.StatusFailed || len(result.Artifacts) != 0 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want failed result without artifacts", result)
	}
	if want := []string{"activate", "capture", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if _, err := store.AcquireLease(result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("profiler lease was not released: %v", err)
	}
}

func TestAuditProfilerRestoresAndReleasesAfterCollectFailure(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{collectErr: errors.New("collect profiler")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err == nil || err.Error() != "collect profiler" {
		t.Fatalf("run error = %v, want collect failure", err)
	}
	if result.Status != audit.StatusFailed || len(result.Artifacts) != 0 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want failed result without artifacts", result)
	}
	if want := []string{"activate", "capture", "collect", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if _, err := store.AcquireLease(result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("profiler lease was not released: %v", err)
	}
}

func TestAuditProfilerRestoresAndReleasesAfterCancellation(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{captureStarted: make(chan struct{})}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runOutcome struct {
		result audit.RunResult
		err    error
	}
	completed := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(ctx, profilerRunRequest())
		completed <- runOutcome{result: result, err: err}
	}()
	<-runtime.captureStarted
	cancel()
	outcome := <-completed
	if outcome.err != nil {
		t.Fatalf("run error = %v, want cancellation represented in the result", outcome.err)
	}
	if outcome.result.Status != audit.StatusCancelled || len(outcome.result.Errors) != 0 || len(outcome.result.Artifacts) != 0 {
		t.Fatalf("result = %#v, want cancelled result without errors or artifacts", outcome.result)
	}
	if want := []string{"activate", "capture", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if _, err := store.AcquireLease(outcome.result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("profiler lease was not released: %v", err)
	}
}

func TestAuditProfilerKeepsLeaseAndSurfacesRestoreFailureAfterCancellation(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{
		captureStarted: make(chan struct{}),
		restoreErr:     errors.New("restore profiler"),
	}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runOutcome struct {
		result audit.RunResult
		err    error
	}
	completed := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(ctx, profilerRunRequest())
		completed <- runOutcome{result: result, err: err}
	}()
	<-runtime.captureStarted
	cancel()
	outcome := <-completed
	if outcome.err == nil || !strings.Contains(outcome.err.Error(), "restore profiler") {
		t.Fatalf("run error = %v, want restore failure", outcome.err)
	}
	if outcome.result.Status != audit.StatusCancelled || len(outcome.result.Errors) != 1 {
		t.Fatalf("result = %#v, want cancelled result with restore failure", outcome.result)
	}
	if want := []string{"activate", "capture", "restore"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want %#v", runtime.events, want)
	}
	if _, err := store.AcquireLease(outcome.result.ProjectID, "diagnostics", "next-owner"); err == nil {
		t.Fatal("profiler lease was released after restore failure")
	}
}

func TestAuditProfilerTimesOutRestoreAndKeepsLease(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{restoreUntilCancel: true, restoreStarted: make(chan struct{})}
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:                  store,
		ProfilerRuntime:        runtime,
		ProfilerCleanupTimeout: 10 * time.Millisecond,
		Resources:              audit.Resources{CPU: 2, MemoryMB: 2048},
	})

	type runOutcome struct {
		result audit.RunResult
		err    error
	}
	completed := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(context.Background(), profilerRunRequest())
		completed <- runOutcome{result: result, err: err}
	}()
	select {
	case outcome := <-completed:
		if outcome.err == nil || !strings.Contains(outcome.err.Error(), "context deadline exceeded") {
			t.Fatalf("run error = %v, want cleanup timeout", outcome.err)
		}
		if outcome.result.Status != audit.StatusFailed || len(outcome.result.Errors) != 1 {
			t.Fatalf("result = %#v, want failed result with cleanup timeout", outcome.result)
		}
		if _, err := store.AcquireLease(outcome.result.ProjectID, "diagnostics", "next-owner"); err == nil {
			t.Fatal("profiler lease was released after restore timeout")
		}
	case <-time.After(time.Second):
		t.Fatal("profiler cleanup did not respect its timeout")
	}
}

func profilerRunRequest() audit.RunRequest {
	request := auditRunRequest()
	request.Checks = []string{"profiler"}
	request.ProfilerURL = "https://shop.test/"
	request.LintJobs = 0
	request.SelectedPHPVersions = nil
	request.MatrixComplete = false
	return request
}

type fakeProfilerRuntime struct {
	events             []string
	activateRequests   []audit.ProfilerRequest
	csv                []byte
	activateErr        error
	captureErr         error
	collectErr         error
	restoreErr         error
	captureStarted     chan struct{}
	restoreStarted     chan struct{}
	restoreUntilCancel bool
}

func (runtime *fakeProfilerRuntime) Activate(_ context.Context, request audit.ProfilerRequest) error {
	runtime.events = append(runtime.events, "activate")
	runtime.activateRequests = append(runtime.activateRequests, request)
	return runtime.activateErr
}

func (runtime *fakeProfilerRuntime) Capture(ctx context.Context, _ audit.ProfilerRequest) error {
	runtime.events = append(runtime.events, "capture")
	if runtime.captureStarted != nil {
		close(runtime.captureStarted)
		<-ctx.Done()
		return ctx.Err()
	}
	return runtime.captureErr
}

func (runtime *fakeProfilerRuntime) Collect(context.Context, audit.ProfilerRequest) ([]byte, error) {
	runtime.events = append(runtime.events, "collect")
	return runtime.csv, runtime.collectErr
}

func (runtime *fakeProfilerRuntime) Restore(ctx context.Context, _ audit.ProfilerRequest) error {
	runtime.events = append(runtime.events, "restore")
	if runtime.restoreStarted != nil {
		close(runtime.restoreStarted)
	}
	if runtime.restoreUntilCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	return runtime.restoreErr
}
