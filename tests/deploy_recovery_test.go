package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/deploy"
)

// A run that does not execute deploy:lock — `--from` past it, or a resume whose
// earlier run recorded it ok — still has to hold the lock for its whole
// duration. Without it the tail's deploy:unlock removes whatever lock sits at
// that path, which may belong to a deploy that is still running.
func TestRecoveryRunCannotStealAnotherRunsLock(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	lock := host.LockPath()
	if err := os.MkdirAll(lock, 0o755); err != nil {
		t.Fatalf("seed the lock: %v", err)
	}
	activated := filepath.Join(host.DeployPath, "activated")
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskLock, Stage: deploy.StagePrepare, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "touch " + activated},
		{ID: deploy.TaskUnlock, Stage: deploy.StageCleanup, Command: "rm -rf " + lock},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	options := deploy.Options{CommandTimeout: time.Minute, From: deploy.TaskActivate}
	_, err = deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("2", "abc", "local"))
	if err == nil {
		t.Fatal("a run that skips deploy:lock must refuse to continue while another run holds the lock")
	}
	if !strings.Contains(err.Error(), "lock") {
		t.Fatalf("the refusal must name the lock, got: %v", err)
	}
	if _, statErr := os.Stat(lock); statErr != nil {
		t.Fatalf("the other run's lock must survive: %v", statErr)
	}
	if _, statErr := os.Stat(activated); statErr == nil {
		t.Fatal("nothing may be activated once the lock is refused")
	}
}

// With no release number there is no release directory: `{{release_path}}`
// resolves to the directory that holds every release, so activation would
// publish all of them at once. A step past prepare must refuse to run.
func TestRunRefusesAPostPrepareStepWithoutAReleaseNumber(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskLock, Stage: deploy.StagePrepare, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "mkdir -p " + host.CurrentPath + " && ln -sfn " + host.ReleasesPath() + " " + host.CurrentPath},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	options := deploy.Options{CommandTimeout: time.Minute, From: deploy.TaskActivate}
	_, err = deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewRelease("", "abc", "local"))
	if err == nil {
		t.Fatal("a publish step with no release number must be refused")
	}
	if !strings.Contains(err.Error(), "release") {
		t.Fatalf("the refusal must name the missing release number, got: %v", err)
	}
	if _, statErr := os.Lstat(host.CurrentPath); statErr == nil {
		t.Fatal("nothing may be activated when the release number is missing")
	}
}

// A resumed run re-takes the lock (`resumeAlwaysReruns` keeps deploy:lock in
// that set, because a resumed run that skipped the lock would proceed
// unprotected) and releases the one it took. The activation step fails unless
// the lock is held while it runs, so this test can tell "took the lock" from
// "never took one".
func TestResumedRunHoldsAndReleasesItsOwnLock(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	lock := host.LockPath()
	activated := filepath.Join(host.DeployPath, "activated")
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskLock, Stage: deploy.StagePrepare, Core: deploy.CoreLock},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "test -d " + lock + " && touch " + activated},
		{ID: deploy.TaskUnlock, Stage: deploy.StageCleanup, Command: "rm -rf " + lock},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	// The record of the failed run: the lock step succeeded, the activation did
	// not, so a resume repeats from the activation.
	release := deploy.NewReleaseForTest("3", "abc", "local")
	release.Tasks = []deploy.StepRecord{{ID: deploy.TaskLock, Status: deploy.StepOK}}
	if err := deploy.WriteRelease(context.Background(), host, release); err != nil {
		t.Fatalf("write the record: %v", err)
	}

	options := deploy.Options{CommandTimeout: time.Minute, Resume: true}
	outcome, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome.LockHeld {
		t.Fatal("a finished resume leaves no lock behind")
	}
	if _, statErr := os.Stat(activated); statErr != nil {
		t.Fatalf("the activation must run with the lock held: %v", statErr)
	}
	if _, statErr := os.Stat(lock); statErr == nil {
		t.Fatal("the resumed run must release the lock it took")
	}
}

// `--from` on its own starts mid-pipeline, so the run has no release number:
// `{{release_path}}` becomes the directory that holds every release and an
// activation publishes all of them. The refusal is precise about where the line
// is: a `--from` that still *runs* `deploy:release` creates the number, so it
// needs no resume — which is the rule the docs state and the code used to be
// broader than.
func TestDeployFromRequiresResumeOnlyPastTheReleaseTask(t *testing.T) {
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "true"},
		{ID: deploy.TaskRelease, Stage: deploy.StagePrepare, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "true"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	if err := cmd.ValidateFromNeedsResumeForTest(plan, deploy.TaskActivate, false); err == nil {
		t.Fatal("--from past deploy:release without --resume must be refused")
	} else if !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("the refusal must name --resume, got: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest(plan, deploy.TaskActivate, true); err != nil {
		t.Fatalf("--from with --resume is the recovery path and must be allowed: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest(plan, deploy.TaskRelease, false); err != nil {
		t.Fatalf("--from deploy:release runs it, so the run has a release number and must be allowed: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest(plan, deploy.TaskCheck, false); err != nil {
		t.Fatalf("--from before deploy:release must be allowed: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest(plan, "", false); err != nil {
		t.Fatalf("a run without --from must not be refused: %v", err)
	}
}

// `--resume` targets the newest unfinished record, and a record that is still
// `running` belongs to a deploy that may be running right now. Resuming it
// releases a live lock and puts two runs on the same release directory,
// including its migrations. Only a lock that is gone or stale makes it ours.
func TestResumeRefusesARunningReleaseUnderAFreshLock(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	if err := os.MkdirAll(host.ReleasePath("2")+"/.dep", 0o755); err != nil {
		t.Fatalf("mkdir the release: %v", err)
	}
	running := deploy.NewReleaseForTest("2", "abc", "main")
	running.Status = deploy.StatusRunning
	if err := deploy.WriteRelease(ctx, host, running); err != nil {
		t.Fatalf("seed the record: %v", err)
	}
	if err := os.MkdirAll(host.LockPath(), 0o755); err != nil {
		t.Fatalf("seed the lock: %v", err)
	}

	release := deploy.NewRelease("", "abc", "main")
	err := cmd.PrepareResumeWithOptionsForTest(ctx, host, release, deploy.Options{LockStaleAfter: time.Hour})
	if err == nil {
		t.Fatal("a running release under a fresh lock must not be resumed")
	}
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("the refusal must name the release, got: %v", err)
	}
	if _, statErr := os.Stat(host.LockPath()); statErr != nil {
		t.Fatal("the refusal must leave the live lock alone")
	}
	// A lock younger than the staleness window needs --force, and the refusal is
	// the only place that says so: `govard deploy unlock` alone is refused for the
	// same reason this is.
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal must name --force for a lock that is not stale yet, got: %v", err)
	}
}

// lockProbeTransportRunner fails the lock probe the way a dead connection does:
// not with `test`'s "no", which is exit 1, but with no answer at all.
type lockProbeTransportRunner struct {
	deploy.Runner
	lockPath string
}

func (r lockProbeTransportRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	if strings.Contains(command, "test -d") && strings.Contains(command, r.lockPath) {
		return deploy.Result{}, fmt.Errorf("%w: the connection died mid-probe", deploy.ErrConnectionMayHaveDropped)
	}
	return r.Runner.Run(ctx, command, opts)
}

// "The lock is not there" and "I could not ask" are different answers, and only
// the first one means the run is ours. A transport error on the probe used to be
// read as "no lock", so a resume released the lock of a deploy that might be
// running right now and put two runs on the same release directory, migrations
// included — the exact outcome the lock exists to prevent.
func TestResumeRefusesWhenTheLockCannotBeChecked(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	if err := os.MkdirAll(host.ReleasePath("2")+"/.dep", 0o755); err != nil {
		t.Fatalf("mkdir the release: %v", err)
	}
	running := deploy.NewReleaseForTest("2", "abc", "main")
	running.Status = deploy.StatusRunning
	if err := deploy.WriteRelease(ctx, host, running); err != nil {
		t.Fatalf("seed the record: %v", err)
	}
	if err := os.MkdirAll(host.LockPath(), 0o755); err != nil {
		t.Fatalf("seed the lock: %v", err)
	}
	blind := host.WithRunner(lockProbeTransportRunner{Runner: host.Runner(), lockPath: host.LockPath()})

	release := deploy.NewRelease("", "abc", "main")
	err := cmd.PrepareResumeWithOptionsForTest(ctx, blind, release, deploy.Options{LockStaleAfter: time.Hour})
	if err == nil {
		t.Fatal("a lock that cannot be checked must not be treated as absent")
	}
	if !strings.Contains(err.Error(), "lock") {
		t.Fatalf("the refusal must name the lock, got: %v", err)
	}
	if _, statErr := os.Stat(host.LockPath()); statErr != nil {
		t.Fatal("a lock that may belong to a live run must survive the refusal")
	}
}

// A failed release below a newer live one is history, not work in progress:
// resuming it would activate an older revision over the live release.
func TestResumeRefusesAReleaseOlderThanTheLiveOne(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	for number, status := range map[string]string{"3": deploy.StatusFailed, "4": deploy.StatusOK} {
		if err := os.MkdirAll(host.ReleasePath(number)+"/.dep", 0o755); err != nil {
			t.Fatalf("mkdir release %s: %v", number, err)
		}
		record := deploy.NewReleaseForTest(number, "rev-"+number, "main")
		record.Status = status
		if err := deploy.WriteRelease(ctx, host, record); err != nil {
			t.Fatalf("seed release %s: %v", number, err)
		}
	}
	if _, err := host.Runner().Run(ctx, "ln -s "+host.ReleasePath("4")+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed the live symlink: %v", err)
	}

	release := deploy.NewRelease("", "rev-3", "main")
	err := cmd.PrepareResumeWithOptionsForTest(ctx, host, release, deploy.Options{LockStaleAfter: time.Hour})
	if err == nil {
		t.Fatal("a release older than the live one must not be resumed")
	}
	if !strings.Contains(err.Error(), "4") {
		t.Fatalf("the refusal must name the live release, got: %v", err)
	}
}

// A failure at or after deploy:activate leaves the failed release as the one the
// target is serving: `current` points at it (and in place the docroot's HEAD is
// its revision). `--resume` exists for exactly that state — a hook, the cache
// flush or the verification failing after the site went live, which is the state
// the executor's own "already deployed" escape hatch was written to stop
// mis-reporting. The guard that refuses to resume *backwards* must refuse an
// older release, never this one: refusing it leaves `deploy unlock --force` and a
// fresh full deploy as the only way to close a maintenance window.
func TestResumeContinuesTheLiveReleaseWhenItIsUnfinished(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	if err := os.MkdirAll(host.ReleasePath("3")+"/.dep", 0o755); err != nil {
		t.Fatalf("mkdir the release: %v", err)
	}
	record := deploy.NewReleaseForTest("3", "rev-3", "main")
	record.Status = deploy.StatusFailed
	if err := deploy.WriteRelease(ctx, host, record); err != nil {
		t.Fatalf("seed the failed record: %v", err)
	}
	if _, err := host.Runner().Run(ctx, "ln -s "+host.ReleasePath("3")+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed the live symlink: %v", err)
	}

	release := deploy.NewRelease("", "rev-3", "main")
	if err := cmd.PrepareResumeWithOptionsForTest(ctx, host, release, deploy.Options{LockStaleAfter: time.Hour}); err != nil {
		t.Fatalf("the live release is the unfinished one, so resuming it is the recovery: %v", err)
	}
	if release.Release != "3" {
		t.Fatalf("the resume must continue release 3, got %q", release.Release)
	}
}

// An in-place rollback runs the executor over the record of the release it is
// rolling back *to*, and the executor rewrites that record as it goes: a failure
// stored `failed` on a release that is perfectly good. That record is what
// `deploy rollback --to <T>` reads to decide which releases are usable, so one
// failed attempt took the healthy release out of every future rollback — the
// operator's way back was destroyed by the attempt to use it. The record is
// restored, and the error names a command that works: the lock belongs to the
// caller and is already released, so telling the operator to unlock is wrong.
func TestAFailedInPlaceRollbackLeavesTheTargetUsable(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	target := deploy.NewReleaseForTest("5", "rev-5", "main")
	target.Status = deploy.StatusOK
	target.Publish.Strategy = deploy.PublishInPlace
	target.Path = host.ReleasePath("5")
	if err := os.MkdirAll(filepath.Join(target.Path, ".dep"), 0o755); err != nil {
		t.Fatalf("mkdir the release: %v", err)
	}
	if err := deploy.WriteRelease(ctx, host, target); err != nil {
		t.Fatalf("seed the target record: %v", err)
	}

	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskMaintenanceEnable, Stage: deploy.StagePublish, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "exit 1"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	err = cmd.RunInPlaceRollbackTailForTest(ctx, host, deploy.Options{CommandTimeout: time.Minute}, deploy.NewVars(),
		plan.ForPublishStrategy(deploy.PublishInPlace), target, io.Discard, false)
	if err == nil {
		t.Fatal("a failing rollback tail must fail")
	}
	if !strings.Contains(err.Error(), "maintenance") {
		t.Errorf("the failure must say the window is still open, got: %v", err)
	}
	if strings.Contains(err.Error(), "deploy unlock") {
		t.Errorf("the caller releases the lock itself, so the hint must not send the operator to unlock: %v", err)
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Errorf("the hint must name the command that finishes the rollback, got: %v", err)
	}

	stored, readErr := deploy.ReadRelease(ctx, host, "5")
	if readErr != nil {
		t.Fatalf("read the target record: %v", readErr)
	}
	if stored.Status != deploy.StatusOK {
		t.Fatalf("a failed rollback must leave the target's record alone, got status %q", stored.Status)
	}
	selected, selErr := deploy.SelectRollbackTargetForTest(ctx, host, "5")
	if selErr != nil {
		t.Fatalf("the healthy release must still be selectable as a rollback target: %v", selErr)
	}
	if selected.Release != "5" {
		t.Fatalf("selected release %q, want 5", selected.Release)
	}
}

// An in-place rollback rewrites the docroot while it serves, exactly as a deploy
// does, so it has to open the window first — the rollback tail starts *after*
// maintenance:enable, which is why shaping the plan alone was not enough. If the
// tail fails the window stays open, the same policy the deploy lock follows.
func TestInPlaceRollbackOpensTheWindowBeforeItsTail(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	log := filepath.Join(t.TempDir(), "window.log")
	echo := func(line string) string { return "echo " + line + " >> " + log }

	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskMaintenanceEnable, Stage: deploy.StagePublish, Command: echo("enable")},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: echo("activate")},
		{ID: deploy.TaskAppCacheFlush, Stage: deploy.StagePublish, Command: echo("flush")},
		{ID: deploy.TaskMaintenanceDisable, Stage: deploy.StagePublish, Command: echo("disable")},
		{ID: deploy.TaskCleanup, Stage: deploy.StageCleanup, Command: echo("cleanup")},
		{ID: deploy.TaskUnlock, Stage: deploy.StageCleanup, Command: echo("unlock")},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	shaped := plan.ForPublishStrategy(deploy.PublishInPlace)
	options := deploy.Options{CommandTimeout: time.Minute}
	if err := cmd.RunInPlaceRollbackTailForTest(context.Background(), host, options, deploy.NewVars(),
		shaped, deploy.NewReleaseForTest("1", "abc", "local"), io.Discard, false); err != nil {
		t.Fatalf("in-place rollback tail: %v", err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read the window log: %v", err)
	}
	order := strings.Fields(strings.TrimSpace(string(raw)))
	want := []string{"enable", "activate", "flush", "disable"}
	if len(order) != len(want) {
		t.Fatalf("window log = %v, want %v (the tail must not carry cleanup or unlock)", order, want)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("window log = %v, want %v", order, want)
		}
	}
}

// With `--with-db` the database is restored after the tail, and the tail's own
// `deploy:verify` queries that database: the framework's checks run
// `setup:db:status` / `migrate:status`, so verifying before the restore compares
// the release being returned to against the schema it is about to replace — and
// fails the rollback for the very state the restore was asked to fix. The tail
// therefore does not verify when a restore follows; the rollback verifies once,
// afterwards.
func TestInPlaceRollbackHoldsBackVerifyWhenARestoreFollows(t *testing.T) {
	log := filepath.Join(t.TempDir(), "tail.log")
	echo := func(line string) string { return "echo " + line + " >> " + log }
	tasks := []deploy.Task{
		{ID: deploy.TaskMaintenanceEnable, Stage: deploy.StagePublish, Command: echo("enable")},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: echo("activate")},
		{ID: deploy.TaskVerify, Stage: deploy.StageVerify, Command: echo("verify")},
		{ID: deploy.TaskMaintenanceDisable, Stage: deploy.StagePublish, Command: echo("disable")},
	}

	for _, testCase := range []struct {
		name             string
		restoreFollows   bool
		wantVerifyInTail bool
	}{
		{name: "without a restore the tail verifies", restoreFollows: false, wantVerifyInTail: true},
		{name: "with a restore the tail holds it back", restoreFollows: true, wantVerifyInTail: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.Remove(log); err != nil && !os.IsNotExist(err) {
				t.Fatalf("clear the log: %v", err)
			}
			host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
			plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", tasks), nil)
			if err != nil {
				t.Fatalf("build plan: %v", err)
			}
			if err := cmd.RunInPlaceRollbackTailForTest(context.Background(), host,
				deploy.Options{CommandTimeout: time.Minute}, deploy.NewVars(),
				plan.ForPublishStrategy(deploy.PublishInPlace),
				deploy.NewReleaseForTest("1", "abc", "local"), io.Discard, testCase.restoreFollows); err != nil {
				t.Fatalf("in-place rollback tail: %v", err)
			}

			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatalf("read the tail log: %v", err)
			}
			ran := strings.Contains(string(raw), "verify")
			if ran != testCase.wantVerifyInTail {
				t.Fatalf("the tail ran verify = %v, want %v (log: %q)", ran, testCase.wantVerifyInTail, raw)
			}
		})
	}
}

// The order restore → flush → verify is the fix, not an accident of layout: the
// caches are rebuilt from the restored database, and the verification queries it.
// A step that does not apply is nil, and a failure stops the chain — the two
// properties the caller relies on when it folds `--with-db` and `--no-verify`
// into the same three closures.
func TestAfterRestoreRunsRestoreThenFlushThenVerify(t *testing.T) {
	var order []string
	step := func(name string) func() error {
		return func() error {
			order = append(order, name)
			return nil
		}
	}
	if err := cmd.AfterRestoreForTest(step("restore"), step("flush"), step("verify")); err != nil {
		t.Fatalf("afterRestore: %v", err)
	}
	if strings.Join(order, ",") != "restore,flush,verify" {
		t.Fatalf("order = %v, want restore, flush, verify", order)
	}

	order = nil
	if err := cmd.AfterRestoreForTest(nil, step("flush"), nil); err != nil {
		t.Fatalf("afterRestore with skipped steps: %v", err)
	}
	if strings.Join(order, ",") != "flush" {
		t.Fatalf("skipped steps must not run: %v", order)
	}

	sentinel := errors.New("restore failed")
	order = nil
	err := cmd.AfterRestoreForTest(func() error { order = append(order, "restore"); return sentinel }, step("flush"), step("verify"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("the failure must surface, got %v", err)
	}
	if strings.Join(order, ",") != "restore" {
		t.Fatalf("a failed restore must stop the chain, ran %v", order)
	}
}
