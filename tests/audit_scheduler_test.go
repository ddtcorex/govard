package tests

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/audit"
)

func TestAuditSchedulerHonorsDependencies(t *testing.T) {
	var mu sync.Mutex
	order := []string{}
	record := func(id string) audit.JobFunc {
		return func(context.Context) (map[string]any, error) {
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
			return map[string]any{"id": id}, nil
		}
	}

	jobs := []audit.Job{
		{ID: "report", DependsOn: []string{"lint"}, Resources: audit.Resources{CPU: 1, MemoryMB: 64}, Run: record("report")},
		{ID: "lint", Resources: audit.Resources{CPU: 2, MemoryMB: 512}, Run: record("lint")},
	}
	results, err := audit.NewScheduler(audit.Resources{CPU: 2, MemoryMB: 512}).Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || order[0] != "lint" || order[1] != "report" {
		t.Fatalf("execution order = %#v", order)
	}
}

func TestAuditSchedulerRunsReadyJobsOnlyWithinBudget(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{})
	var thirdStarted atomic.Bool
	blocking := func(id string) audit.JobFunc {
		return func(context.Context) (map[string]any, error) {
			started <- id
			<-release
			return nil, nil
		}
	}

	jobs := []audit.Job{
		{ID: "first", Resources: audit.Resources{CPU: 1, MemoryMB: 256}, Run: blocking("first")},
		{ID: "second", Resources: audit.Resources{CPU: 1, MemoryMB: 256}, Run: blocking("second")},
		{ID: "third", Resources: audit.Resources{CPU: 1, MemoryMB: 256}, Run: func(context.Context) (map[string]any, error) {
			thirdStarted.Store(true)
			started <- "third"
			return nil, nil
		}},
	}

	done := make(chan error, 1)
	go func() {
		_, err := audit.NewScheduler(audit.Resources{CPU: 2, MemoryMB: 512}).Run(context.Background(), jobs)
		done <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatalf("ready jobs did not start concurrently: %#v", seen)
		}
	}
	if !seen["first"] || !seen["second"] || thirdStarted.Load() {
		t.Fatalf("started before release = %#v, third=%t", seen, thirdStarted.Load())
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not finish after resources were released")
	}
}

func TestAuditSchedulerRejectsResourceAllocationOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, test := range []struct {
		name      string
		allocated audit.Resources
		request   audit.Resources
	}{
		{
			name:      "CPU",
			allocated: audit.Resources{CPU: maxInt},
			request:   audit.Resources{CPU: 1},
		},
		{
			name:      "memory",
			allocated: audit.Resources{MemoryMB: maxInt},
			request:   audit.Resources{MemoryMB: 1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if audit.CanAllocateResourcesForTest(audit.Resources{CPU: maxInt, MemoryMB: maxInt}, test.allocated, test.request) {
				t.Fatal("resource request was accepted after capacity was fully allocated")
			}
		})
	}
}

func TestAuditSchedulerNormalizesZeroResources(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var secondStarted atomic.Bool
	jobs := []audit.Job{
		{ID: "first", Run: func(context.Context) (map[string]any, error) {
			close(firstStarted)
			<-releaseFirst
			return nil, nil
		}},
		{ID: "second", Run: func(context.Context) (map[string]any, error) {
			secondStarted.Store(true)
			return nil, nil
		}},
	}

	done := make(chan error, 1)
	go func() {
		_, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), jobs)
		done <- err
	}()
	select {
	case <-firstStarted:
		if secondStarted.Load() {
			t.Fatal("zero-resource job bypassed the scheduler budget")
		}
		close(releaseFirst)
	case <-time.After(time.Second):
		t.Fatal("first job did not start")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAuditSchedulerRejectsInvalidGraphsBeforeStartingJobs(t *testing.T) {
	for name, jobs := range map[string][]audit.Job{
		"cycle": {
			{ID: "first", DependsOn: []string{"second"}},
			{ID: "second", DependsOn: []string{"first"}},
		},
		"unknown dependency": {
			{ID: "first", DependsOn: []string{"missing"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var started atomic.Bool
			for i := range jobs {
				jobs[i].Run = func(context.Context) (map[string]any, error) {
					started.Store(true)
					return nil, nil
				}
			}
			_, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), jobs)
			if err == nil {
				t.Fatal("Run accepted an invalid job graph")
			}
			if started.Load() {
				t.Fatal("Run started a job before validating the graph")
			}
		})
	}
}

func TestAuditSchedulerRejectsInvalidJobDefinitionsBeforeStartingJobs(t *testing.T) {
	run := func(context.Context) (map[string]any, error) { return nil, nil }
	for _, test := range []struct {
		name string
		jobs []audit.Job
	}{
		{
			name: "empty ID",
			jobs: []audit.Job{{ID: "", Run: run}},
		},
		{
			name: "duplicate ID",
			jobs: []audit.Job{{ID: "duplicate", Run: run}, {ID: "duplicate", Run: run}},
		},
		{
			name: "negative resources",
			jobs: []audit.Job{{ID: "negative", Resources: audit.Resources{CPU: -1}, Run: run}},
		},
		{
			name: "over budget",
			jobs: []audit.Job{{ID: "oversized", Resources: audit.Resources{CPU: 2, MemoryMB: 1}, Run: run}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), test.jobs)
			if err == nil {
				t.Fatal("Run accepted an invalid job definition")
			}
		})
	}
}

func TestAuditSchedulerCancellationStopsPendingJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var pendingStarted atomic.Bool
	jobs := []audit.Job{
		{ID: "running", Resources: audit.Resources{CPU: 1, MemoryMB: 1}, Run: func(ctx context.Context) (map[string]any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}},
		{ID: "pending", Resources: audit.Resources{CPU: 1, MemoryMB: 1}, Run: func(context.Context) (map[string]any, error) {
			pendingStarted.Store(true)
			return nil, nil
		}},
	}

	done := make(chan []audit.JobResult, 1)
	go func() {
		results, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(ctx, jobs)
		if err != nil {
			t.Errorf("Run error = %v", err)
		}
		done <- results
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("running job did not start")
	}

	select {
	case results := <-done:
		if pendingStarted.Load() {
			t.Fatal("pending job started after context cancellation")
		}
		if results[0].Status != audit.StatusCancelled || results[1].Status != audit.StatusCancelled {
			t.Fatalf("statuses = %q, %q, want cancelled", results[0].Status, results[1].Status)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not wait for the running job to exit")
	}
}

func TestAuditSchedulerReturnsResultsInInputOrder(t *testing.T) {
	fastFinished := make(chan struct{})
	jobs := []audit.Job{
		{ID: "slow", Resources: audit.Resources{CPU: 1, MemoryMB: 1}, Run: func(context.Context) (map[string]any, error) {
			<-fastFinished
			return nil, nil
		}},
		{ID: "fast", Resources: audit.Resources{CPU: 1, MemoryMB: 1}, Run: func(context.Context) (map[string]any, error) {
			close(fastFinished)
			return nil, nil
		}},
	}

	results, err := audit.NewScheduler(audit.Resources{CPU: 2, MemoryMB: 2}).Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ID != "slow" || results[1].ID != "fast" {
		t.Fatalf("result IDs = %#v, want input order", []string{results[0].ID, results[1].ID})
	}
}

func TestAuditSchedulerCancelsTransitiveDependentsAfterFailure(t *testing.T) {
	var dependentStarted atomic.Bool
	jobs := []audit.Job{
		{ID: "failed", Run: func(context.Context) (map[string]any, error) { return nil, errors.New("lint failed") }},
		{ID: "dependent", DependsOn: []string{"failed"}, Run: func(context.Context) (map[string]any, error) {
			dependentStarted.Store(true)
			return nil, nil
		}},
		{ID: "transitive", DependsOn: []string{"dependent"}, Run: func(context.Context) (map[string]any, error) {
			dependentStarted.Store(true)
			return nil, nil
		}},
	}

	results, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	if dependentStarted.Load() {
		t.Fatal("dependent job ran after a dependency failed")
	}
	if results[0].Status != audit.StatusFailed || results[1].Status != audit.StatusCancelled || results[2].Status != audit.StatusCancelled {
		t.Fatalf("statuses = %#v", []audit.Status{results[0].Status, results[1].Status, results[2].Status})
	}
	if results[0].Evidence["error"] != "lint failed" {
		t.Fatalf("failed evidence = %#v", results[0].Evidence)
	}
}

func TestAuditSchedulerContinuesIndependentBranchAfterFailure(t *testing.T) {
	independentRan := make(chan struct{})
	var dependentRan atomic.Bool
	jobs := []audit.Job{
		{ID: "failed", Run: func(context.Context) (map[string]any, error) { return nil, errors.New("lint failed") }},
		{ID: "dependent", DependsOn: []string{"failed"}, Run: func(context.Context) (map[string]any, error) {
			dependentRan.Store(true)
			return nil, nil
		}},
		{ID: "independent", Run: func(context.Context) (map[string]any, error) {
			close(independentRan)
			return nil, nil
		}},
	}

	results, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-independentRan:
	default:
		t.Fatal("independent job did not run after another branch failed")
	}
	if dependentRan.Load() {
		t.Fatal("dependent job ran after a dependency failed")
	}
	if results[0].Status != audit.StatusFailed || results[1].Status != audit.StatusCancelled || results[2].Status != audit.StatusPassed {
		t.Fatalf("statuses = %#v", []audit.Status{results[0].Status, results[1].Status, results[2].Status})
	}
}

func TestAuditSchedulerCancelsFailedBranchAndRunsIndependentWorkAfterCapacityReleases(t *testing.T) {
	failedStarted := make(chan struct{})
	allowFailure := make(chan struct{})
	independentStarted := make(chan struct{})
	var downstreamStarted atomic.Bool
	jobs := []audit.Job{
		{ID: "failed", Run: func(context.Context) (map[string]any, error) {
			close(failedStarted)
			<-allowFailure
			return nil, errors.New("lint failed")
		}},
		{ID: "dependent", DependsOn: []string{"failed"}, Run: func(context.Context) (map[string]any, error) {
			downstreamStarted.Store(true)
			return nil, nil
		}},
		{ID: "transitive", DependsOn: []string{"dependent"}, Run: func(context.Context) (map[string]any, error) {
			downstreamStarted.Store(true)
			return nil, nil
		}},
		{ID: "independent", Run: func(context.Context) (map[string]any, error) {
			close(independentStarted)
			return nil, nil
		}},
	}

	done := make(chan []audit.JobResult, 1)
	go func() {
		results, err := audit.NewScheduler(audit.Resources{CPU: 1, MemoryMB: 1}).Run(context.Background(), jobs)
		if err != nil {
			t.Errorf("Run error = %v", err)
		}
		done <- results
	}()
	select {
	case <-failedStarted:
		select {
		case <-independentStarted:
			t.Fatal("independent job started before failed work released capacity")
		default:
		}
		close(allowFailure)
	case <-time.After(time.Second):
		t.Fatal("failing job did not start")
	}
	select {
	case <-independentStarted:
	case <-time.After(time.Second):
		t.Fatal("independent job did not start after failed work released capacity")
	}
	results := <-done
	if downstreamStarted.Load() {
		t.Fatal("a downstream job started after its dependency failed")
	}
	if results[0].Status != audit.StatusFailed || results[1].Status != audit.StatusCancelled || results[2].Status != audit.StatusCancelled || results[3].Status != audit.StatusPassed {
		t.Fatalf("statuses = %#v", []audit.Status{results[0].Status, results[1].Status, results[2].Status, results[3].Status})
	}
}
