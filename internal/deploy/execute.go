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
	Steps   []StepResult
	Release string
	Total   time.Duration
	// LockHeld reports whether the deploy lock is still held on the target
	// after this run. A failure before publish releases it; from publish
	// onwards it survives, so recovery has to be explicit.
	LockHeld        bool
	AlreadyDeployed bool
}

// LockKeptOnFailure reports whether a failure in this stage must keep the deploy
// lock (spec 7.3).
//
// Prepare and build only produced local state — a release directory, a mirror,
// build output — so releasing the lock is safe and a plain retry must not be
// refused by a lock nothing is using. From publish onwards the target may have
// been changed (maintenance enabled, code reset, schema migrated), so the lock
// stays and recovery is an explicit `--resume` or `deploy unlock --force`.
func LockKeptOnFailure(stage Stage) bool {
	switch stage {
	case StagePrepare, StageBuild:
		return false
	default:
		return true
	}
}

// RecoveryHint names the command that actually recovers a failed deploy.
//
// The two failures need different commands: a run holding the lock cannot be
// retried — the retry is refused — while one that released its lock only needs
// the fault fixed (spec P4, 7.3).
func RecoveryHint(remote string, lockHeld bool) string {
	if lockHeld {
		return fmt.Sprintf("the release directory, its record and the deploy lock were kept on %s; continue with `govard deploy %s --resume`, or inspect the target with `govard deploy status %s`", remote, remote, remote)
	}
	return fmt.Sprintf("nothing live changed and the deploy lock was released; fix the reported error and retry `govard deploy %s`", remote)
}

// releaseLockAfterFailure removes the lock a failed pre-publish run still holds.
//
// It only removes a lock this run took. A run that never acquired one — a
// rejected preflight, or a plan with locking disabled — has nothing to release,
// and removing a lock another deploy holds would be worse than leaving this
// one behind.
func (e *Executor) releaseLockAfterFailure(ctx context.Context, release *Release) {
	if !e.lockWasAcquired() {
		return
	}
	sc := StepContext{
		Host:    e.host,
		Runner:  e.host.Runner(),
		Vars:    NewVars(),
		Release: release,
		Opts:    e.opts,
		Out:     e.out,
	}
	// The deploy context may be cancelled by the failure; releasing the lock is
	// cleanup and must still happen.
	if err := CoreUnlock(context.WithoutCancel(ctx), &sc); err != nil {
		fmt.Fprintf(e.out, "  ! the deploy lock could not be released: %v\n", err)
		return
	}
	fmt.Fprintf(e.out, "  lock released (failure before publish; nothing live changed)\n")
}

// lockWasAcquired reports whether the lock step of this run succeeded.
func (e *Executor) lockWasAcquired() bool {
	for _, result := range e.results {
		if result.ID == TaskLock {
			return result.Status == StepOK
		}
	}
	return false
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

	// Said before anything runs, so it is on screen when the deploy fails later
	// for the reason it warns about.
	if warning := MissingVerifyWarning(plan, e.opts); warning != "" {
		fmt.Fprintf(e.out, "  ! WARNING: %s\n", warning)
	}

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

	// Which release is live right now is what `{{previous_release}}` and the
	// record's `publish.previous_release` mean, and it has to be read before
	// activation replaces it. Best effort: not knowing is not a reason to stop.
	if release.Publish.PreviousRelease == "" {
		if live := liveReleaseName(ctx, e.host); live != "" && live != release.Release {
			release.Publish.PreviousRelease = live
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

	window := plan.maintenanceWindow()
	timeoutFor := func(index int) time.Duration {
		if window[index] {
			if e.opts.MaintenanceTimeout > 0 {
				return e.opts.MaintenanceTimeout
			}
			return DefaultMaintenanceTimeout
		}
		if e.opts.CommandTimeout > 0 {
			return e.opts.CommandTimeout
		}
		return DefaultCommandTimeout
	}

	for index, step := range plan.Steps {
		if completed[step.ID] {
			e.record(ctx, release, step, StepSkipped, 0, nil)
			continue
		}
		// A step the plan marked skipped is not run, whatever it carries. The
		// build-mode branch is decided at plan-build time and shows up only as
		// `Skipped`, so an executor that ignored the flag would run the server
		// build in artifact mode as well.
		if step.Skipped || (step.Command == "" && step.core == nil) {
			e.record(ctx, release, step, StepSkipped, 0, nil)
			continue
		}
		// Locking is the one policy the run decides for itself rather than the
		// plan, because it comes from a flag and not from a build mode. Both
		// lock steps go together: taking no lock and then removing whatever is
		// at the lock path would delete the lock of a deploy still running.
		if e.opts.SkipLock && (step.ID == TaskLock || step.ID == TaskUnlock) {
			step.SkipReason = "locking is disabled"
			e.record(ctx, release, step, StepSkipped, 0, nil)
			continue
		}

		// Release-derived variables must reflect what earlier steps produced,
		// so `{{release_path}}` is correct in every command after deploy:release.
		stepVars := vars.Set("release", release.Release)
		if release.Path != "" {
			stepVars = stepVars.SetPath("release_path", release.Path)
		}
		if release.Publish.PreviousRelease != "" {
			stepVars = stepVars.SetPath("previous_release", e.host.ReleasePath(release.Publish.PreviousRelease))
		}

		stepCtx := StepContext{
			Host:    e.host,
			Runner:  e.host.Runner(),
			Vars:    stepVars,
			Release: release,
			Opts:    e.opts,
			Out:     e.out,
			WorkDir: e.workDir,
			Checks:  step.Checks,
		}

		stepStarted := time.Now()
		stepErr := e.runStep(ctx, step, stepCtx, timeoutFor(index))
		elapsed := time.Since(stepStarted)

		if stepErr != nil && !step.Optional {
			e.record(ctx, release, step, StepFailed, elapsed, stepErr)
			release.Status = StatusFailed
			// Best effort: the failure that matters is the step's, not the
			// record write.
			_ = WriteRelease(context.WithoutCancel(ctx), e.host, release)
			outcome.LockHeld = LockKeptOnFailure(step.Stage)
			if !outcome.LockHeld {
				e.releaseLockAfterFailure(ctx, release)
			}
			return e.finish(outcome, started), fmt.Errorf("step %s failed on %s: %w", step.ID, e.host.Name, stepErr)
		}

		e.record(ctx, release, step, StepOK, elapsed, nil)
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
// command otherwise. Both are bounded, because an unbounded remote command is how
// a deploy hangs; `timeout` is the bound the caller chose for this step, which is
// smaller inside a maintenance window (spec 7.3).
func (e *Executor) runStep(ctx context.Context, step Step, stepCtx StepContext, timeout time.Duration) error {
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

// record appends a result, mirrors it into the release record, and stores the
// record on the target.
//
// Storing after every step is what spec 6 asks for: `deploy status` and
// monitoring read that file while a deploy is running, and a run that dies in
// the build stage otherwise leaves no evidence of how far it got. The write is
// skipped until the release number exists — before `deploy:release` there is no
// release directory to write into, and creating one early would be worse than
// writing late.
func (e *Executor) record(ctx context.Context, release *Release, step Step, status string, duration time.Duration, err error) {
	result := StepResult{ID: step.ID, Stage: step.Stage, Status: status, Duration: duration, Err: err}
	e.results = append(e.results, result)

	record := StepRecord{ID: step.ID, Status: status, DurationMS: duration.Milliseconds()}
	if err != nil {
		record.Error = err.Error()
	}
	release.RecordTask(record)

	if release.Release != "" {
		// Never the run's context: a failed step may have cancelled it, and the
		// record is exactly what the operator needs afterwards.
		if writeErr := WriteRelease(context.WithoutCancel(ctx), e.host, release); writeErr != nil {
			fmt.Fprintf(e.out, "  ! the release record could not be updated: %v\n", writeErr)
		}
	}

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

// ShortRevision is the eight-character form of a revision, which is what a
// human reads in a timeline, a static content version or a log line. Git's
// abbreviations are longer because they have to stay unique across a repository;
// this one is only ever a label or a cache-busting token.
func ShortRevision(revision string) string {
	if len(revision) > 8 {
		return revision[:8]
	}
	return revision
}

// shortRevision is ShortRevision with a label for a missing revision, which is
// how the timeline renders one.
func shortRevision(revision string) string {
	if revision == "" {
		return "unknown"
	}
	return ShortRevision(revision)
}
