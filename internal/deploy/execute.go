package deploy

import (
	"context"
	"errors"
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
	Steps []StepResult
	Total time.Duration
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
//
// The remote is named with `--remote` rather than as a positional argument.
// One form that is correct for every remote beats a per-remote special case:
// the hint never has to be kept in step with the command tree, and it reads
// the same whether the remote is configured or synthetic.
func RecoveryHint(remote string, lockHeld bool) string {
	if lockHeld {
		return fmt.Sprintf("the release directory, its record and the deploy lock were kept on %s; continue with `govard deploy --remote %s --resume`, or inspect the target with `govard deploy status %s`", remote, remote, remote)
	}
	return fmt.Sprintf("nothing live changed and the deploy lock was released; fix the reported error and retry `govard deploy --remote %s`", remote)
}

// releaseLockAfterFailure removes the lock a failed pre-publish run still holds.
//
// It only removes a lock this run took. A run that never acquired one — a
// rejected preflight, or a plan with locking disabled — has nothing to release,
// and removing a lock another deploy holds would be worse than leaving this
// one behind.
func (e *Executor) releaseLockAfterFailure(ctx context.Context, release *Release) {
	if !e.lockHeld {
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

// ComposerAuthEnv is the environment variable a CI job or a server sets to give
// Composer credentials for private repositories. Govard passes it through and
// never stores it: it is not written to the release record, not put in a command
// and not kept on the target.
const ComposerAuthEnv = "COMPOSER_AUTH"

// Executor runs one plan against one host.
type Executor struct {
	host         Host
	opts         Options
	out          io.Writer
	composerAuth string
	results      []StepResult
	// workDir is the checkout a step inspects (the .gitmodules probe).
	workDir string
	// lockHeld is whether this run holds the deploy lock right now. It is set
	// by the deploy:lock step when it runs, and by Run itself when `--from` or a
	// resume skipped that step: a run that is about to touch the target has to
	// hold the lock, and the tail's deploy:unlock may only remove a lock this
	// run took.
	lockHeld bool
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
	return &Executor{
		host:    host,
		opts:    opts,
		out:     out,
		workDir: workDir,
		// Read once, at construction: the value never enters Options or the
		// release record, so it cannot be logged or persisted by accident.
		composerAuth: strings.TrimSpace(os.Getenv(ComposerAuthEnv)),
	}
}

// migrationSkipReason is the timeline reason for a task the probe excused. It
// names the probe, not a framework command, because the engine never knows
// which framework it runs for.
const migrationSkipReason = "db up-to-date (probe exit 0)"

// Run executes every step in order and stops at the first failure that is not
// marked optional.
//
// The bookkeeping writes go through context.WithoutCancel: a failed step may
// have cancelled the deploy context, and the release record is exactly what the
// operator needs afterwards.
func (e *Executor) Run(ctx context.Context, plan Plan, vars Vars, release *Release) (Outcome, error) {
	started := time.Now()
	outcome := Outcome{}

	if release.Path == "" {
		release.Path = e.host.ReleasePath(release.Release)
	}
	if release.Branch == "" {
		release.Branch = e.opts.Branch
	}

	fmt.Fprintf(e.out, "▶ deploy %s (%s @ %s)\n", e.host.Name, BranchLabel(release.Branch), shortRevision(release.Revision))

	// Said before anything runs, so it is on screen when the deploy fails later
	// for the reason it warns about.
	if warning := MissingVerifyWarning(plan, e.opts); warning != "" {
		fmt.Fprintf(e.out, "  ! WARNING: %s\n", warning)
	}

	// No-op fast path: a target already running the requested revision, with a
	// *complete* record behind it, is not deployed again. Without it a CI retry
	// would rebuild and re-publish an identical release. --force overrides.
	//
	// The completeness half matters: a release whose record says failed or still
	// running served the revision at some point, and reporting "already
	// deployed" for it would turn a retry into a success that never happened.
	//
	// A resumed run is exempt for the same reason, one step further on. It exists
	// to finish an unfinished release, and the question this shortcut asks — "is
	// the target serving this revision" — can be answered yes by an *older*
	// successful release of the same revision while the unfinished one is mid-way
	// through its maintenance window. Observed live: a hook failed after
	// activation, the target answered 503 with the window open, and the resume
	// that was supposed to close it printed "already deployed" in 1.3s and exited
	// 0 — a success reported over a site that was down.
	continuing := e.opts.Resume && strings.TrimSpace(release.Release) != ""
	if !e.opts.Force && !continuing && strings.TrimSpace(release.Revision) != "" {
		if live, complete, ok := e.liveRevision(ctx); ok && complete && live == release.Revision {
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
	// carried holds the record entry of every step an earlier run of this
	// release already succeeded at. The run does not repeat the step, and it
	// must not rewrite the entry either: `ok` is what the next resume reads to
	// decide what not to repeat, so recording the carry-over as `skipped`
	// destroys the very fact that makes resuming safe.
	carried := map[string]StepRecord{}
	storedMigrateVerdict := false
	if e.opts.Resume && release.Release != "" {
		if previous, err := ReadRelease(ctx, e.host, release.Release); err == nil {
			for _, task := range previous.Tasks {
				if task.Status != StepOK || resumeAlwaysReruns[task.ID] {
					continue
				}
				completed[task.ID] = true
				carried[task.ID] = task
			}
			if previous.Revision != "" {
				release.Revision = previous.Revision
			}
			// A recorded migrate verdict is sticky: the database was drifted
			// when the earlier run probed it, and migrations are idempotent,
			// so re-running one is always safe. A recorded skip is NOT
			// adopted — drift may have appeared out of band since, and an
			// old skip would publish code over a drifted database.
			if previous.Migration != nil && previous.Migration.Required {
				storedMigrateVerdict = true
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

	// A run that does not execute deploy:lock — `--from` past it, or a resume
	// whose earlier run recorded it ok — still has to hold the lock for its whole
	// duration. The run is about to touch the target, and the tail's
	// deploy:unlock removes whatever sits at the lock path: without taking the
	// lock here, that `rm -rf` deletes the lock of a deploy that is still
	// running. Taking it also means a partial run is refused while somebody else
	// holds the lock, the same guarantee a full run gets.
	if completed[TaskLock] && !e.opts.SkipLock {
		lockCtx := &StepContext{
			Host:    e.host,
			Runner:  e.host.Runner(),
			Vars:    vars,
			Release: release,
			Opts:    e.opts,
			Out:     e.out,
		}
		if err := CoreLock(ctx, lockCtx); err != nil {
			return e.finish(outcome, started), fmt.Errorf("acquire the deploy lock: %w", err)
		}
		e.lockHeld = true
	}

	// The migration probe resolves lazily, at the first gated step that would
	// otherwise run — not upfront. The release directory does not exist until
	// deploy:release creates it, and the probe starts with
	// `cd {{release_path}}`: running it earlier makes the cd fail, whose exit
	// 1 reads as "drifted", so every deploy would migrate. A resumed run
	// adopts a recorded migrate verdict instead of re-probing — the drift it
	// records may have been healed out of band since, and a fresh probe would
	// then excuse the teardown the first run already started — while a
	// recorded skip is never adopted, because drift may equally have appeared
	// since. Steps an earlier run recorded as ok are carried over below, so a
	// resume never repeats a migration that already succeeded. When every
	// gated step was already carried over, the probe never runs because there
	// is nothing to decide.
	migrationRequired := true
	probeDone := storedMigrateVerdict
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
			if stored, ok := carried[step.ID]; ok {
				e.carryOver(ctx, release, step, stored)
			} else {
				// `--from` put this step behind the run without an earlier run
				// having succeeded at it, so there is no success to keep.
				e.record(ctx, release, step, StepSkipped, 0, nil)
			}
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
		// The probe resolves here, at the first gated step that would otherwise
		// run: everything it asks about — the release directory, the code, the
		// shared links — exists by now. A probe failure stops the deploy like a
		// step failure, with the lock policy of the stage it failed in.
		if step.NeedsMigration && !probeDone && plan.MigrationProbe != nil {
			probeDone = true
			required, probeExit, err := e.checkMigrationRequired(ctx, plan, vars, release)
			if err != nil {
				release.Status = StatusFailed
				_ = WriteRelease(context.WithoutCancel(ctx), e.host, release)
				outcome.LockHeld = LockKeptOnFailure(step.Stage)
				if !outcome.LockHeld {
					e.releaseLockAfterFailure(ctx, release)
				}
				return e.finish(outcome, started), err
			}
			migrationRequired = required
			// The verdict is stored the moment it resolves, not with the next
			// step's record: a resume must see it even when the very next
			// step is the one that fails. Best effort, like every other
			// mid-run record write — the failure below keeps its own write.
			release.Migration = &MigrationRecord{
				Required:   required,
				ProbeExit:  probeExit,
				ResolvedAt: time.Now().UTC().Format(time.RFC3339),
			}
			_ = WriteRelease(context.WithoutCancel(ctx), e.host, release)
			if !migrationRequired && plan.windowIsGated() {
				// No window ever opens: the static plan still lists the
				// maintenance tasks, but the runtime knows they will all be
				// skipped, and the in-window timeout must not apply to
				// anything from here on. timeoutFor reads this per step, so
				// reassigning mid-loop is safe. In place the window opens
				// whatever the probe answered, so the smaller bound stays: the
				// activation that rewrites the docroot is bounded by how long
				// the site may stay down, not by the command timeout.
				window = nil
			}
		}
		// A step the probe excused is not run either. The timeline still shows
		// it, with the reason the engine — not the framework — supplies.
		if step.NeedsMigration && !migrationRequired {
			step.SkipReason = migrationSkipReason
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
		// The same rule from the other side: a run that holds no lock must not
		// remove one. Reachable when a plan carries deploy:unlock without
		// deploy:lock, or after a lock acquisition that never happened.
		if step.ID == TaskUnlock && !e.lockHeld {
			step.SkipReason = "this run did not hold the deploy lock"
			e.record(ctx, release, step, StepSkipped, 0, nil)
			continue
		}

		// A release number is what `{{release_path}}` is built from. Without one
		// that variable is the directory holding every release, so an activation
		// would publish all of them at once, and the record would be written
		// beside them. deploy:release sets it; a run that skipped that step has
		// nothing to continue and must stop before it touches the target.
		if release.Release == "" && step.Stage != StagePrepare {
			stepErr := fmt.Errorf("step %s needs a release number and deploy:release did not run in this run; pass --resume to continue a release that already exists", step.ID)
			e.record(ctx, release, step, StepFailed, 0, stepErr)
			release.Status = StatusFailed
			_ = WriteRelease(context.WithoutCancel(ctx), e.host, release)
			outcome.LockHeld = LockKeptOnFailure(step.Stage)
			if !outcome.LockHeld {
				e.releaseLockAfterFailure(ctx, release)
			}
			return e.finish(outcome, started), fmt.Errorf("step %s failed on %s: %w", step.ID, e.host.Name, stepErr)
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

		// One writer per step, shared by the command's output and the heartbeat: the
		// heartbeat takes its lock, so a tick can never land inside a half-written
		// line. It only becomes the command's Out when the operator asked to watch,
		// which is what keeps the default run's output identical.
		live := newLiveWriter(e.out, linePrefix)
		stepCtx := StepContext{
			Host:     e.host,
			Runner:   e.host.Runner(),
			Vars:     stepVars,
			Release:  release,
			Opts:     e.opts,
			Out:      e.out,
			WorkDir:  e.workDir,
			Checks:   step.Checks,
			Terminal: isTerminal(e.out),
		}
		if e.opts.Verbose && !e.opts.JSON {
			stepCtx.Live = live
		}

		stepStarted := time.Now()
		stepErr := e.runStep(ctx, step, stepCtx, timeoutFor(index), live)
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
		if step.ID == TaskLock {
			e.lockHeld = true
		}
	}

	release.Status = StatusOK
	if err := WriteRelease(context.WithoutCancel(ctx), e.host, release); err != nil {
		return e.finish(outcome, started), err
	}
	return e.finish(outcome, started), nil
}

// checkMigrationRequired runs the plan's migration probe once and reports
// whether the NeedsMigration tasks must run. Only the exit code is read: 0
// means the target is current, 1 or 2 means it drifted. Anything else — an
// unexpected exit, a timeout, a transport error — fails the run rather than
// guessing, because a skipped migration on a drifted database breaks the site
// while a failed probe merely stops the deploy.
func (e *Executor) checkMigrationRequired(ctx context.Context, plan Plan, vars Vars, release *Release) (bool, int, error) {
	probe := plan.MigrationProbe
	probeVars := vars.Set("release", release.Release)
	probePath := release.Path
	if probePath == "" {
		probePath = e.host.ReleasePath(release.Release)
	}
	probeVars = probeVars.SetPath("release_path", probePath)
	if release.Publish.PreviousRelease != "" {
		probeVars = probeVars.SetPath("previous_release", e.host.ReleasePath(release.Publish.PreviousRelease))
	}
	command, err := probeVars.Expand(probe.Command)
	if err != nil {
		return false, 0, fmt.Errorf("expand the migration probe: %w", err)
	}
	timeout := e.opts.CommandTimeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	result, err := e.host.Runner().Run(ctx, command, RunOptions{Timeout: timeout})
	code := result.ExitCode
	if err != nil {
		var cmdErr *CommandError
		if !errors.As(err, &cmdErr) {
			return false, 0, fmt.Errorf("migration probe failed on %s: %w", e.host.Name, err)
		}
		code = cmdErr.ExitCode
	}
	switch code {
	case 0:
		if err != nil {
			return false, code, fmt.Errorf("migration probe reported success with an error on %s: %w", e.host.Name, err)
		}
		return false, code, nil
	case 1, 2:
		return true, code, nil
	default:
		return false, code, fmt.Errorf("migration probe exited %d on %s (want 0 = current, 1/2 = migrate); refusing to guess: %s", code, e.host.Name, command)
	}
}

// liveRevision reports the revision the target is serving, whether the record
// behind it is complete, and whether either could be determined at all. It reads
// the current release's record for a symlink layout and the docroot's HEAD for an
// in-place one.
//
// "Complete" means the record says the release finished: an unfinished or failed
// release may still be serving its revision, and the caller must not treat that
// as a deployment that is already in place.
func (e *Executor) liveRevision(ctx context.Context) (string, bool, bool) {
	runner := e.host.Runner()

	if result, err := runner.Run(ctx, "readlink -f "+Shell(e.host.CurrentPath), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		resolved := strings.TrimSpace(result.Stdout)
		if resolved != "" && resolved != e.host.CurrentPath {
			if record, err := ReadRelease(ctx, e.host, path.Base(resolved)); err == nil && record.Revision != "" {
				return record.Revision, record.Status == StatusOK, true
			}
		}
	}

	if result, err := runner.Run(ctx, "git -C "+Shell(e.host.CurrentPath)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout}); err == nil {
		if revision := strings.TrimSpace(result.Stdout); revision != "" {
			// An in-place docroot has no record of its own, so completeness is
			// whether some release govard recorded for that revision finished.
			return revision, e.hasCompleteRecordFor(ctx, revision), true
		}
	}
	return "", false, false
}

// hasCompleteRecordFor reports whether a finished release was recorded for this
// revision.
func (e *Executor) hasCompleteRecordFor(ctx context.Context, revision string) bool {
	entries, err := ListReleases(ctx, e.host)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Foreign || entry.Status != StatusOK {
			continue
		}
		if record, err := ReadRelease(ctx, e.host, entry.Release); err == nil && record.Revision == revision {
			return true
		}
	}
	return false
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
func (e *Executor) runStep(ctx context.Context, step Step, stepCtx StepContext, timeout time.Duration, live *liveWriter) error {
	// The heartbeat covers every step, core or shell: a step whose command prints
	// nothing is otherwise indistinguishable from a hung one until the timeout.
	stopHeartbeat := startHeartbeat(step, live)
	defer stopHeartbeat()

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
	runOptions := RunOptions{Timeout: timeout, Out: stepCtx.Live}
	if step.ID == TaskVendors && step.RunOn != RunLocal && e.composerAuth != "" {
		command = ComposerAuthCommand(command)
		runOptions.Stdin = e.composerAuth
	}
	_, err = runner.Run(stepCtxRun, command, runOptions)
	return err
}

// ComposerAuthCommand makes a command read its credentials from standard input
// into COMPOSER_AUTH.
//
// Standard input is the only channel that keeps the secret out of everything that
// outlives the run: argv is visible to `ps` on the target, the command text is
// printed by --verbose and captured in CI logs, and a file on the target is a
// credential govard promised not to store.
func ComposerAuthCommand(command string) string {
	return ComposerAuthEnv + `="$(cat)"; export ` + ComposerAuthEnv + `; ` + command
}

// ComposerAuthCommandForTest exposes ComposerAuthCommand to the tests/ package.
func ComposerAuthCommandForTest(command string) string {
	return ComposerAuthCommand(command)
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

	e.printStep(step, status, duration)
}

// carryOver records a step a resumed run does not repeat because an earlier run
// of the same release already succeeded at it.
//
// The run's own outcome says `skipped` — it did not run, and its timeline has to
// say so. The durable record keeps the `ok` the earlier run stored, because the
// record describes the release rather than this run, and a resume reads it to
// decide what not to repeat. Recording the carry-over as `skipped` made a second
// resume run the step again, and for `deploy:release` — which refuses a
// directory that already exists — the second resume could then never succeed:
// the directory was there, the step could not run, and the record said the step
// had never succeeded.
func (e *Executor) carryOver(ctx context.Context, release *Release, step Step, stored StepRecord) {
	e.results = append(e.results, StepResult{ID: step.ID, Stage: step.Stage, Status: StepSkipped})

	release.RecordTask(stored)
	if release.Release != "" {
		if writeErr := WriteRelease(context.WithoutCancel(ctx), e.host, release); writeErr != nil {
			fmt.Fprintf(e.out, "  ! the release record could not be updated: %v\n", writeErr)
		}
	}

	step.SkipReason = "already done in an earlier run"
	e.printStep(step, StepSkipped, 0)
}

// printStep writes one line of the run timeline. It is shared by every path that
// records a step so the timeline cannot drift from the record.
func (e *Executor) printStep(step Step, status string, duration time.Duration) {
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

// BranchLabel renders a branch for a human line, naming the detached state
// instead of printing nothing. The plan renderer shares it so both outputs
// spell the state the same way.
func BranchLabel(branch string) string {
	if strings.TrimSpace(branch) == "" {
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
