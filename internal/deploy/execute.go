package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

// Step outcomes.
const (
	StepOK      = "ok"
	StepSkipped = "skipped"
	StepFailed  = "failed"
)

// PublishStage is the stage after which a failure must keep the lock: the
// target may already be mid-change, so a second deploy must not start silently.
const PublishStage = StagePublish

// resumeAlwaysReruns lists the idempotent, safety-relevant steps a resumed
// deploy must repeat. Skipping the lock would let a resumed run proceed
// unprotected, which defeats the point of the lock.
var resumeAlwaysReruns = map[string]bool{
	TaskCheck:  true,
	TaskLock:   true,
	TaskUnlock: true,
}

// StepResult is the outcome of one executed (or skipped) step.
type StepResult struct {
	ID       string
	Stage    Stage
	Status   string
	Duration time.Duration
	Err      error
}

// Outcome is what one `Run` produced.
type Outcome struct {
	Steps           []StepResult
	Release         string
	Total           time.Duration
	LockHeld        bool
	AlreadyDeployed bool
}

// Executor runs one plan against one host.
type Executor struct {
	host    Host
	opts    Options
	out     io.Writer
	results []StepResult
	// workDir is the checkout a step inspects (the .gitmodules probe).
	workDir string
}

// NewExecutor builds an executor. out receives the stage timeline; pass
// io.Discard to keep the pipeline silent (tests, --json).
func NewExecutor(host Host, opts Options, out io.Writer) *Executor {
	if out == nil {
		out = io.Discard
	}
	workDir, err := os.Getwd()
	if err != nil {
		workDir = ""
	}
	return &Executor{host: host, opts: opts, out: out, workDir: workDir}
}

// Run executes every step in order and stops at the first failure that is not
// marked optional.
//
// The bookkeeping writes go through context.WithoutCancel: a failed step may
// have cancelled the deploy context, and the release record is exactly what the
// operator needs afterwards.
func (e *Executor) Run(ctx context.Context, plan Plan, vars Vars, release *Release) (Outcome, error) {
	started := time.Now()
	outcome := Outcome{Release: release.Release}

	if release.Path == "" {
		release.Path = e.host.ReleasePath(release.Release)
	}
	if release.Branch == "" {
		release.Branch = e.opts.Branch
	}
	if release.Repository == "" {
		release.Repository = e.opts.Repository
	}

	fmt.Fprintf(e.out, "▶ deploy %s (%s @ %s)\n", e.host.Name, branchLabel(release.Branch), shortRevision(release.Revision))

	// No-op fast path: a target already running the requested revision is not
	// deployed again. Without it a CI retry would rebuild and re-publish an
	// identical release. --force overrides.
	if !e.opts.Force && strings.TrimSpace(release.Revision) != "" {
		if live, ok := e.liveRevision(ctx); ok && live == release.Revision {
			outcome.AlreadyDeployed = true
			fmt.Fprintf(e.out, "  = already deployed %s\n", shortRevision(release.Revision))
			return e.finish(outcome, started), nil
		}
	}

	// A resumed deploy repeats only what did not succeed: a task already
	// recorded as ok is not run twice.
	completed := map[string]bool{}
	if e.opts.Resume && release.Release != "" {
		if previous, err := ReadRelease(ctx, e.host, release.Release); err == nil {
			for _, task := range previous.Tasks {
				if task.Status != StepOK || resumeAlwaysReruns[task.ID] {
					continue
				}
				completed[task.ID] = true
			}
			if previous.Revision != "" {
				release.Revision = previous.Revision
			}
		}
	}

	// --from names the first step to run. Everything before it is recorded as
	// skipped rather than dropped, so the timeline still shows the whole
	// pipeline and an operator can see what was deliberately not repeated.
	if e.opts.From != "" {
		if index := plan.IndexOf(e.opts.From); index > 0 {
			for _, step := range plan.Steps[:index] {
				completed[step.ID] = true
			}
		}
	}

	for _, step := range plan.Steps {
		if completed[step.ID] {
			e.record(release, step, StepSkipped, 0, nil)
			continue
		}
		// A step the plan marked skipped is not run, whatever it carries. The
		// build-mode branch is decided at plan-build time and shows up only as
		// `Skipped`, so an executor that ignored the flag would run the server
		// build in artifact mode as well.
		if step.Skipped || (step.Command == "" && step.core == nil) {
			e.record(release, step, StepSkipped, 0, nil)
			continue
		}

		// Release-derived variables must reflect what earlier steps produced,
		// so `{{release_path}}` is correct in every command after deploy:release.
		stepVars := vars.Set("release", release.Release)
		if release.Path != "" {
			stepVars = stepVars.SetPath("release_path", release.Path)
		}

		stepCtx := StepContext{
			Host:    e.host,
			Runner:  e.host.Runner(),
			Vars:    stepVars,
			Release: release,
			Opts:    e.opts,
			Out:     e.out,
			WorkDir: e.workDir,
		}

		stepStarted := time.Now()
		stepErr := e.runStep(ctx, step, stepCtx)
		elapsed := time.Since(stepStarted)

		if stepErr != nil && !step.Optional {
			e.record(release, step, StepFailed, elapsed, stepErr)
			release.Status = StatusFailed
			// Best effort: the failure that matters is the step's, not the
			// record write.
			_ = WriteRelease(context.WithoutCancel(ctx), e.host, release)
			outcome.LockHeld = step.Stage == PublishStage
			return e.finish(outcome, started), fmt.Errorf("step %s failed on %s: %w", step.ID, e.host.Name, stepErr)
		}

		e.record(release, step, StepOK, elapsed, nil)
	}

	release.Status = StatusOK
	if err := WriteRelease(context.WithoutCancel(ctx), e.host, release); err != nil {
		return e.finish(outcome, started), err
	}
	return e.finish(outcome, started), nil
}

// liveRevision reports the revision the target is currently serving, and
// whether it could be determined at all. It reads the current release's record
// for a symlink layout and the docroot's HEAD for an in-place one.
func (e *Executor) liveRevision(ctx context.Context) (string, bool) {
	runner := e.host.Runner()

	if result, err := runner.Run(ctx, "readlink -f "+Shell(e.host.CurrentPath), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		resolved := strings.TrimSpace(result.Stdout)
		if resolved != "" && resolved != e.host.CurrentPath {
			if record, err := ReadRelease(ctx, e.host, path.Base(resolved)); err == nil && record.Revision != "" {
				return record.Revision, true
			}
		}
	}

	if result, err := runner.Run(ctx, "git -C "+Shell(e.host.CurrentPath)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout}); err == nil {
		if revision := strings.TrimSpace(result.Stdout); revision != "" {
			return revision, true
		}
	}
	return "", false
}

func (e *Executor) finish(outcome Outcome, started time.Time) Outcome {
	outcome.Steps = e.results
	outcome.Total = time.Since(started)
	fmt.Fprintf(e.out, "  total %s\n", outcome.Total.Round(time.Millisecond))
	return outcome
}

// runStep executes one step: a Go implementation when the task has one, a shell
// command otherwise. Both are bounded by the configured command timeout, because
// an unbounded remote command is how a deploy hangs.
func (e *Executor) runStep(ctx context.Context, step Step, stepCtx StepContext) error {
	timeout := e.opts.CommandTimeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	stepCtxRun := ctx
	if step.RunOn != RunLocal {
		var cancel context.CancelFunc
		stepCtxRun, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if step.core != nil {
		return step.core(stepCtxRun, &stepCtx)
	}

	command, err := stepCtx.Vars.Expand(step.Command)
	if err != nil {
		return fmt.Errorf("expand %s: %w", step.ID, err)
	}
	runner := stepCtx.Runner
	if step.RunOn == RunLocal {
		runner = LocalRunner{}
	}
	_, err = runner.Run(stepCtxRun, command, RunOptions{Timeout: timeout})
	return err
}

// record appends a result and mirrors it into the release record.
func (e *Executor) record(release *Release, step Step, status string, duration time.Duration, err error) {
	result := StepResult{ID: step.ID, Stage: step.Stage, Status: status, Duration: duration, Err: err}
	e.results = append(e.results, result)

	record := StepRecord{ID: step.ID, Status: status, DurationMS: duration.Milliseconds()}
	if err != nil {
		record.Error = err.Error()
	}
	release.RecordTask(record)

	icon := "✔"
	switch status {
	case StepSkipped:
		icon = "–"
	case StepFailed:
		icon = "✖"
	}
	title := step.Title
	if status == StepSkipped && step.SkipReason != "" {
		title += " — " + step.SkipReason
	}
	fmt.Fprintf(e.out, "  %s %-22s %8s  %s\n", icon, step.ID, duration.Round(time.Millisecond), title)
}

func branchLabel(branch string) string {
	if branch == "" {
		return "detached"
	}
	return branch
}

func shortRevision(revision string) string {
	if len(revision) > 8 {
		return revision[:8]
	}
	if revision == "" {
		return "unknown"
	}
	return revision
}
