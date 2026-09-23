package tests

import (
	"context"
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

// `--from` on its own starts after deploy:release, so the run has no release
// number: `{{release_path}}` becomes the directory that holds every release and
// an activation publishes all of them. The CLI refuses it and names the fix.
func TestDeployFromRequiresResume(t *testing.T) {
	if err := cmd.ValidateFromNeedsResumeForTest("publish:activate", false); err == nil {
		t.Fatal("--from without --resume must be refused")
	} else if !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("the refusal must name --resume, got: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest("publish:activate", true); err != nil {
		t.Fatalf("--from with --resume is the recovery path and must be allowed: %v", err)
	}
	if err := cmd.ValidateFromNeedsResumeForTest("", false); err != nil {
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
