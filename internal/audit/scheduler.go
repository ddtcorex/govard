package audit

import (
	"context"
	"fmt"
	"time"
)

// Resources describes the capacity a job needs or a scheduler can provide.
type Resources struct {
	CPU      int
	MemoryMB int
}

// JobFunc executes one audit job and returns its evidence.
type JobFunc func(context.Context) (map[string]any, error)

// Job is a unit of audit work with dependencies and resource requirements.
type Job struct {
	ID        string
	Kind      string
	DependsOn []string
	Resources Resources
	Run       JobFunc
}

// Scheduler runs audit jobs while respecting dependency and resource limits.
type Scheduler struct {
	budget Resources
	now    func() time.Time
}

// NewScheduler creates a scheduler with the supplied resource budget.
func NewScheduler(budget Resources) *Scheduler {
	return &Scheduler{
		budget: budget,
		now:    time.Now,
	}
}

// Run executes a validated job graph. Job failures are represented in their
// results; only invalid scheduler inputs return an error.
func (s *Scheduler) Run(ctx context.Context, jobs []Job) ([]JobResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("audit scheduler context is nil")
	}
	budget, jobResources, err := s.validate(jobs)
	if err != nil {
		return nil, err
	}

	results := make([]JobResult, len(jobs))
	for index, job := range jobs {
		results[index] = JobResult{
			ID:     job.ID,
			Kind:   job.Kind,
			Status: StatusPending,
		}
	}
	if len(jobs) == 0 {
		return results, nil
	}

	type completion struct {
		index     int
		evidence  map[string]any
		err       error
		finished  time.Time
		cancelled bool
	}
	completed := make(chan completion, len(jobs))
	allocated := Resources{}
	running := 0
	contextCancelled := false

	start := func(index int) {
		job := jobs[index]
		resources := jobResources[index]
		results[index].Status = StatusRunning
		results[index].StartedAt = s.now().UTC()
		allocated.CPU += resources.CPU
		allocated.MemoryMB += resources.MemoryMB
		running++
		go func() {
			evidence, runErr := job.Run(ctx)
			completed <- completion{
				index:     index,
				evidence:  evidence,
				err:       runErr,
				finished:  s.now().UTC(),
				cancelled: ctx.Err() != nil,
			}
		}()
	}

	for {
		if !contextCancelled && ctx.Err() != nil {
			contextCancelled = true
			markPendingCancelled(results)
		}
		if !contextCancelled {
			markBlockedJobsCancelled(results, jobs)
			for index, job := range jobs {
				if results[index].Status != StatusPending || !dependenciesPassed(results, job.DependsOn, jobs) {
					continue
				}
				resources := jobResources[index]
				if !canAllocateResources(budget, allocated, resources) {
					continue
				}
				start(index)
			}
		}

		if running == 0 {
			if allJobsTerminal(results) {
				return results, nil
			}
			return nil, fmt.Errorf("audit scheduler could not run remaining jobs within resource budget")
		}

		completion := <-completed
		resources := jobResources[completion.index]
		allocated.CPU -= resources.CPU
		allocated.MemoryMB -= resources.MemoryMB
		running--

		result := &results[completion.index]
		result.FinishedAt = completion.finished
		result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
		result.Evidence = completion.evidence
		if completion.cancelled {
			result.Status = StatusCancelled
		} else if completion.err != nil {
			result.Status = StatusFailed
			result.Evidence = withErrorEvidence(completion.evidence, completion.err)
		} else {
			result.Status = StatusPassed
		}
	}
}

func (s *Scheduler) validate(jobs []Job) (Resources, []Resources, error) {
	if s == nil {
		return Resources{}, nil, fmt.Errorf("audit scheduler is nil")
	}
	if s.budget.CPU < 0 || s.budget.MemoryMB < 0 {
		return Resources{}, nil, fmt.Errorf("audit scheduler budget cannot be negative")
	}
	budget := normalizedResources(s.budget)
	indices := make(map[string]int, len(jobs))
	resources := make([]Resources, len(jobs))
	for index, job := range jobs {
		if job.ID == "" {
			return Resources{}, nil, fmt.Errorf("audit job at index %d has an empty ID", index)
		}
		if _, exists := indices[job.ID]; exists {
			return Resources{}, nil, fmt.Errorf("audit job ID %q is duplicated", job.ID)
		}
		if job.Resources.CPU < 0 || job.Resources.MemoryMB < 0 {
			return Resources{}, nil, fmt.Errorf("audit job %q has negative resources", job.ID)
		}
		if job.Run == nil {
			return Resources{}, nil, fmt.Errorf("audit job %q has no runner", job.ID)
		}
		indices[job.ID] = index
		resources[index] = normalizedResources(job.Resources)
		if resources[index].CPU > budget.CPU || resources[index].MemoryMB > budget.MemoryMB {
			return Resources{}, nil, fmt.Errorf("audit job %q exceeds scheduler resource budget", job.ID)
		}
	}
	for _, job := range jobs {
		for _, dependency := range job.DependsOn {
			if _, exists := indices[dependency]; !exists {
				return Resources{}, nil, fmt.Errorf("audit job %q depends on unknown job %q", job.ID, dependency)
			}
		}
	}
	if hasDependencyCycle(jobs, indices) {
		return Resources{}, nil, fmt.Errorf("audit job graph contains a dependency cycle")
	}
	return budget, resources, nil
}

func normalizedResources(resources Resources) Resources {
	if resources.CPU == 0 {
		resources.CPU = 1
	}
	if resources.MemoryMB == 0 {
		resources.MemoryMB = 1
	}
	return resources
}

func canAllocateResources(budget, allocated, request Resources) bool {
	return request.CPU <= budget.CPU-allocated.CPU && request.MemoryMB <= budget.MemoryMB-allocated.MemoryMB
}

// CanAllocateResourcesForTest exposes scheduler admission for external tests.
func CanAllocateResourcesForTest(budget, allocated, request Resources) bool {
	return canAllocateResources(budget, allocated, request)
}

func hasDependencyCycle(jobs []Job, indices map[string]int) bool {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make([]int, len(jobs))
	var visit func(int) bool
	visit = func(index int) bool {
		if state[index] == visiting {
			return true
		}
		if state[index] == visited {
			return false
		}
		state[index] = visiting
		for _, dependency := range jobs[index].DependsOn {
			if visit(indices[dependency]) {
				return true
			}
		}
		state[index] = visited
		return false
	}
	for index := range jobs {
		if visit(index) {
			return true
		}
	}
	return false
}

func dependenciesPassed(results []JobResult, dependencies []string, jobs []Job) bool {
	for _, dependency := range dependencies {
		for index, job := range jobs {
			if job.ID == dependency && results[index].Status != StatusPassed {
				return false
			}
		}
	}
	return true
}

func markBlockedJobsCancelled(results []JobResult, jobs []Job) {
	changed := true
	for changed {
		changed = false
		for index, job := range jobs {
			if results[index].Status != StatusPending {
				continue
			}
			for _, dependency := range job.DependsOn {
				for dependencyIndex, dependencyJob := range jobs {
					if dependencyJob.ID != dependency {
						continue
					}
					if results[dependencyIndex].Status == StatusFailed || results[dependencyIndex].Status == StatusCancelled {
						results[index].Status = StatusCancelled
						changed = true
					}
				}
			}
		}
	}
}

func markPendingCancelled(results []JobResult) {
	for index := range results {
		if results[index].Status == StatusPending {
			results[index].Status = StatusCancelled
		}
	}
}

func allJobsTerminal(results []JobResult) bool {
	for _, result := range results {
		if result.Status == StatusPending || result.Status == StatusRunning {
			return false
		}
	}
	return true
}

func withErrorEvidence(evidence map[string]any, err error) map[string]any {
	result := make(map[string]any, len(evidence)+1)
	for key, value := range evidence {
		result[key] = value
	}
	result["error"] = err.Error()
	return result
}
