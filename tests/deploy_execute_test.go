package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func executorForTest(t *testing.T, tasks []deploy.Task) (deploy.Host, deploy.Plan) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", tasks), nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	return host, plan
}

func TestExecutorRunsStepsInOrderAndRecordsStatus(t *testing.T) {
	host, plan := executorForTest(t, []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "echo one"},
		{ID: deploy.TaskCompile, Stage: deploy.StageBuild}, // no command: skipped
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "echo three"},
	})

	release := deploy.NewReleaseForTest("1", "abc", "local")
	executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
	outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(outcome.Steps) != 3 {
		t.Fatalf("recorded %d steps, want 3", len(outcome.Steps))
	}
	if outcome.Steps[1].Status != deploy.StepSkipped {
		t.Fatalf("empty-command step status = %q, want skipped", outcome.Steps[1].Status)
	}
	if outcome.Steps[0].Status != deploy.StepOK || outcome.Steps[2].Status != deploy.StepOK {
		t.Fatalf("statuses = %+v, want ok/skipped/ok", outcome.Steps)
	}

	stored, err := deploy.ReadRelease(context.Background(), host, "1")
	if err != nil {
		t.Fatalf("read release: %v", err)
	}
	if stored.Status != deploy.StatusOK {
		t.Fatalf("release status = %q, want ok", stored.Status)
	}
	if len(stored.Tasks) != 3 {
		t.Fatalf("release recorded %d tasks, want 3", len(stored.Tasks))
	}
}

func TestExecutorStopsAtTheFirstFailureAndKeepsTheLockInPublish(t *testing.T) {
	host, plan := executorForTest(t, []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "exit 3"},
		{ID: deploy.TaskVerify, Stage: deploy.StageVerify, Command: "echo never"},
	})
	executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
	outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err == nil {
		t.Fatal("want an error from the failing publish step")
	}
	if len(outcome.Steps) != 2 {
		t.Fatalf("ran %d steps, want 2 (the verify step must not run)", len(outcome.Steps))
	}
	if !outcome.LockHeld {
		t.Fatal("a publish-stage failure must keep the lock so a second deploy cannot start")
	}

	stored, readErr := deploy.ReadRelease(context.Background(), host, "1")
	if readErr != nil {
		t.Fatalf("read release: %v", readErr)
	}
	if stored.Status != deploy.StatusFailed {
		t.Fatalf("release status = %q, want failed", stored.Status)
	}
}

// lockRecipe builds a plan that takes the real lock and then fails at exactly
// one task. Every other step is a no-op, so the failure stage is the only
// variable.
func lockRecipe(t *testing.T, failing string) (deploy.Host, deploy.Plan) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	recipe := deploy.DefaultRecipe()
	for _, id := range deploy.TaskIDList() {
		// The two lock steps keep their real implementations: what happens to
		// the lock directory is the thing under test.
		if id == deploy.TaskLock || id == deploy.TaskUnlock || id == failing {
			continue
		}
		stage, _ := deploy.StageForTask(id)
		deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: "true"})
	}
	if failing != "" {
		stage, _ := deploy.StageForTask(failing)
		deploy.OverrideTaskForTest(&recipe, failing, deploy.Task{ID: failing, Stage: stage, Command: "exit 9"})
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	return host, plan
}

// The lock policy is stage-dependent (spec 7.3): a failure before publish
// releases it, because nothing live has changed and the next attempt must not
// be blocked by a lock nothing is using.
func TestExecutorReleasesTheLockWhenAFailureHappensBeforePublish(t *testing.T) {
	for _, failing := range []string{deploy.TaskCheck, deploy.TaskRelease, deploy.TaskCode, deploy.TaskVendors} {
		t.Run(failing, func(t *testing.T) {
			host, plan := lockRecipe(t, failing)
			executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
			outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
			if err == nil {
				t.Fatal("want an error")
			}
			if outcome.LockHeld {
				t.Fatal("a prepare/build failure must release the lock: nothing live has changed yet")
			}
			if _, err := host.Runner().Run(context.Background(), "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
				t.Fatalf("the lock directory must be gone after a %s failure: %v", failing, err)
			}
		})
	}
}

func TestExecutorKeepsTheLockWhenAPublishFailureHappens(t *testing.T) {
	for _, failing := range []string{deploy.TaskMaintenanceEnable, deploy.TaskDBMigrate, deploy.TaskActivate} {
		t.Run(failing, func(t *testing.T) {
			host, plan := lockRecipe(t, failing)
			executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
			outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
			if err == nil {
				t.Fatal("want an error")
			}
			if !outcome.LockHeld {
				t.Fatal("a publish-stage failure must keep the lock: the target may be half-migrated")
			}
			if _, err := host.Runner().Run(context.Background(), "test -d "+host.LockPath(), deploy.RunOptions{}); err != nil {
				t.Fatalf("the lock directory must survive a %s failure: %v", failing, err)
			}
		})
	}
}

func TestLockPolicyFollowsTheFailureStage(t *testing.T) {
	for stage, kept := range map[deploy.Stage]bool{
		deploy.StagePrepare: false,
		deploy.StageBuild:   false,
		deploy.StagePublish: true,
		deploy.StageVerify:  true,
		deploy.StageCleanup: true,
	} {
		if got := deploy.LockKeptOnFailure(stage); got != kept {
			t.Errorf("LockKeptOnFailure(%s) = %v, want %v", stage, got, kept)
		}
	}
}

// The printed recovery hint has to name the command that actually works: a run
// holding the lock cannot simply be retried, because the retry is refused.
func TestRecoveryHintNamesTheCommandThatWorks(t *testing.T) {
	resumed := deploy.RecoveryHint("production", true)
	if !strings.Contains(resumed, "--resume") {
		t.Errorf("a kept lock must be recovered with --resume, got %q", resumed)
	}
	retried := deploy.RecoveryHint("production", false)
	if strings.Contains(retried, "--resume") {
		t.Errorf("a released lock needs a plain retry, not a resume, got %q", retried)
	}
	if !strings.Contains(retried, "retry") {
		t.Errorf("the hint must name the retry, got %q", retried)
	}
}

func TestExecutorTreatsOptionalFailuresAsNonFatal(t *testing.T) {
	host, plan := executorForTest(t, []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "exit 5", Optional: true},
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "true"},
	})
	executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
	outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err != nil {
		t.Fatalf("an optional failure must not fail the deploy: %v", err)
	}
	if len(outcome.Steps) != 2 {
		t.Fatalf("ran %d steps, want 2", len(outcome.Steps))
	}
}

func TestExecutorFromSkipsEverythingBeforeTheNamedTask(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := func(name string) string { return "touch " + filepath.Join(host.DeployPath, name) }

	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: marker("ran-check")},
		{ID: deploy.TaskCode, Stage: deploy.StagePrepare, Command: marker("ran-code")},
		{ID: deploy.TaskDBMigrate, Stage: deploy.StagePublish, Command: marker("ran-migrate")},
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: marker("ran-record")},
	}), nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	options := deploy.Options{Remote: "local", CommandTimeout: time.Minute, From: deploy.TaskDBMigrate}
	executor := deploy.NewExecutor(host, options, io.Discard)
	outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := map[string]string{
		deploy.TaskCheck:     deploy.StepSkipped,
		deploy.TaskCode:      deploy.StepSkipped,
		deploy.TaskDBMigrate: deploy.StepOK,
		deploy.TaskRecord:    deploy.StepOK,
	}
	for _, step := range outcome.Steps {
		if want[step.ID] != step.Status {
			t.Errorf("step %s status = %q, want %q", step.ID, step.Status, want[step.ID])
		}
	}

	// Skipped means skipped: the earlier commands must not have run at all.
	for _, name := range []string{"ran-check", "ran-code"} {
		if _, err := os.Stat(filepath.Join(host.DeployPath, name)); err == nil {
			t.Errorf("--from ran %s, which is before the named task", name)
		}
	}
	for _, name := range []string{"ran-migrate", "ran-record"} {
		if _, err := os.Stat(filepath.Join(host.DeployPath, name)); err != nil {
			t.Errorf("--from did not run %s: %v", name, err)
		}
	}
}

// A step the plan marked skipped must not run. `ForBuildMode` marks the five
// build tasks skipped in artifact mode but leaves their command in place, so a
// skip that lived only in the plan would still run `composer install` and
// `setup:di:compile` on the target — exactly what artifact mode exists to move
// off the production box.
func TestExecutorDoesNotRunAStepThePlanMarkedSkipped(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "build-ran")
	host, plan := executorForTest(t, []deploy.Task{
		{ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "touch " + marker},
		{ID: deploy.TaskDBMigrate, Stage: deploy.StagePublish, Command: "true"},
	})
	plan = plan.ForBuildMode(deploy.BuildArtifact)

	options := deploy.Options{Remote: "local", Build: deploy.BuildArtifact, CommandTimeout: time.Minute}
	outcome, err := deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("artifact mode ran build:vendors, which the plan marked skipped")
	}
	for _, step := range outcome.Steps {
		if step.ID == deploy.TaskVendors && step.Status != deploy.StepSkipped {
			t.Fatalf("build:vendors status = %q, want %q", step.Status, deploy.StepSkipped)
		}
		if step.ID == deploy.TaskDBMigrate && step.Status != deploy.StepOK {
			t.Fatalf("db:migrate status = %q, want %q (only the marked step may be skipped)", step.Status, deploy.StepOK)
		}
	}
}

// Locking is on unless the operator turns it off, and the zero value of the
// option must not be able to turn it off by accident.
func TestLockingIsOnWhenTheOptionIsNotSet(t *testing.T) {
	host, plan := lockRecipe(t, "")
	outcome, err := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard).
		Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	status := map[string]string{}
	for _, step := range outcome.Steps {
		status[step.ID] = step.Status
	}
	if status[deploy.TaskLock] != deploy.StepOK {
		t.Fatalf("deploy:lock = %q, want %q", status[deploy.TaskLock], deploy.StepOK)
	}
	if status[deploy.TaskUnlock] != deploy.StepOK {
		t.Fatalf("deploy:unlock = %q, want %q", status[deploy.TaskUnlock], deploy.StepOK)
	}
	if _, err := host.Runner().Run(context.Background(), "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("the lock must be released at the end of a successful run: %v", err)
	}
}

// `--lock=false` skips both lock steps. Skipping only the acquisition would be
// worse than ignoring the flag: the run would finish by removing whatever lock
// it found, including one belonging to a deploy that is still running.
func TestDisabledLockingTakesNoLockAndLeavesOtherLocksAlone(t *testing.T) {
	host, plan := lockRecipe(t, "")
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed a concurrent deploy's lock: %v", err)
	}

	outcome, err := deploy.NewExecutor(host, deploy.Options{Remote: "local", SkipLock: true, CommandTimeout: time.Minute}, io.Discard).
		Run(ctx, plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err != nil {
		t.Fatalf("a run with locking disabled must not be refused by an existing lock: %v", err)
	}
	status := map[string]string{}
	for _, step := range outcome.Steps {
		status[step.ID] = step.Status
	}
	for _, id := range []string{deploy.TaskLock, deploy.TaskUnlock} {
		if status[id] != deploy.StepSkipped {
			t.Errorf("%s = %q, want %q with locking disabled", id, status[id], deploy.StepSkipped)
		}
	}
	if _, err := host.Runner().Run(ctx, "test -d "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("a disabled-lock run removed a lock it never took: %v", err)
	}
}

// The warning has to reach the operator's screen, not just exist as a function.
func TestExecutorPrintsTheMissingVerifyWarning(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskDBMigrate, deploy.Task{
		ID: deploy.TaskDBMigrate, Stage: deploy.StagePublish, Command: "true",
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "production")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var out bytes.Buffer
	options := deploy.Options{Remote: "local", CommandTimeout: time.Minute, From: deploy.TaskDBMigrate}
	if _, err := deploy.NewExecutor(host, options, &out).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local")); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "deploy.verify.url") {
		t.Fatalf("the run must print the warning, got:\n%s", out.String())
	}
}

// Spec 7.3: steps inside the maintenance window get their own, much smaller
// limit, because a slow step there is holding the site down. Outside the window
// the ordinary command timeout applies.
func TestExecutorBoundsInWindowStepsMoreTightly(t *testing.T) {
	build := func() (deploy.Host, deploy.Plan) {
		recipe := deploy.DefaultRecipe()
		for _, id := range deploy.TaskIDList() {
			stage, _ := deploy.StageForTask(id)
			command := "true"
			switch id {
			case deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable, deploy.TaskDBMigrate:
				// The migration is the step whose timeout is under test.
			default:
				deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: command})
				continue
			}
			if id == deploy.TaskDBMigrate {
				command = "sleep 1"
			}
			deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: command})
		}
		plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		return deploy.HostForTest(t.TempDir(), deploy.LocalRunner{}), plan
	}

	t.Run("the window budget applies inside", func(t *testing.T) {
		host, plan := build()
		options := deploy.Options{
			Remote: "local", CommandTimeout: time.Minute, MaintenanceTimeout: 300 * time.Millisecond,
		}
		_, err := deploy.NewExecutor(host, options, io.Discard).Run(
			context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
		if err == nil {
			t.Fatal("a step inside the window must be bounded by the maintenance timeout")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})

	t.Run("the command budget applies when no window opens", func(t *testing.T) {
		recipe := deploy.RecipeForTest("test", []deploy.Task{
			{ID: deploy.TaskDBMigrate, Stage: deploy.StagePublish, Command: "sleep 1"},
		})
		plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
		options := deploy.Options{
			Remote: "local", CommandTimeout: 30 * time.Second, MaintenanceTimeout: 300 * time.Millisecond,
		}
		if _, err := deploy.NewExecutor(host, options, io.Discard).Run(
			context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local")); err != nil {
			t.Fatalf("without maintenance:enable the command timeout applies: %v", err)
		}
	})
}

// Spec 6: `release.json` "is rewritten after every task, so it is always written
// to release.json.tmp and mv-ed into place — the same write-then-rename
// discipline … because deploy status and monitoring may read it mid-deploy".
// Nothing observed it mid-deploy, so a run that died in the build stage left no
// evidence of how far it had got.
func TestExecutorKeepsTheReleaseRecordCurrent(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	recordPath := host.ReleaseRecordPath("1")
	snapshot := filepath.Join(t.TempDir(), "record-seen-from-deploy-code")

	recipe := deploy.DefaultRecipe()
	for _, id := range deploy.TaskIDList() {
		stage, _ := deploy.StageForTask(id)
		switch id {
		case deploy.TaskLock, deploy.TaskRelease:
			// Real implementations: the lock, and the release number every
			// later write depends on.
		case deploy.TaskCode:
			// Runs after deploy:release, so it can observe what the record holds
			// at that moment.
			deploy.OverrideTaskForTest(&recipe, id, deploy.Task{
				ID: id, Stage: stage, Command: "cp " + recordPath + " " + snapshot,
			})
		default:
			deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: "true"})
		}
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	options := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("", "abc", "local")); err != nil {
		t.Fatalf("run: %v", err)
	}

	seen, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatalf("deploy:code could not read the record: %v", err)
	}
	for _, want := range []string{`"deploy:release"`, `"deploy:lock"`, `"status":"running"`} {
		if !strings.Contains(string(seen), want) {
			t.Fatalf("the record read mid-deploy does not contain %s:\n%s", want, seen)
		}
	}
	// The step that is running is not in the record yet: the write happens as
	// each step completes, which is what makes the file a progress log.
	if strings.Contains(string(seen), `"deploy:code"`) {
		t.Fatalf("the record already claimed deploy:code before it finished:\n%s", seen)
	}
}

// Spec 6 shows `publish.previous_release` populated and spec 7.5 lists
// `{{previous_release}}` as a variable available to recipe commands. Neither was
// ever filled in: the variable was always empty and the field never written, so a
// recipe could not address the release it was replacing.
func TestExecutorRecordsThePreviousReleaseAndExposesItsPath(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	// Release 1 is live, through the current symlink.
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("1")+"/.dep", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}
	live := deploy.NewReleaseForTest("1", "aaa", "main")
	live.Status = deploy.StatusOK
	if err := deploy.WriteRelease(ctx, host, live); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	if _, err := runner.Run(ctx, "ln -s "+host.ReleasePath("1")+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	marker := filepath.Join(t.TempDir(), "previous-release")
	recipe := deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "echo {{previous_release}} > " + marker},
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	options := deploy.Options{Remote: "local", Publish: deploy.PublishSymlink, CommandTimeout: time.Minute}
	release := deploy.NewReleaseForTest("2", "bbb", "main")
	release.Path = host.ReleasePath("2")
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("run: %v", err)
	}

	if release.Publish.PreviousRelease != "1" {
		t.Fatalf("publish.previous_release = %q, want 1", release.Publish.PreviousRelease)
	}
	seen, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if got := strings.Trim(strings.TrimSpace(string(seen)), "'"); got != host.ReleasePath("1") {
		t.Fatalf("{{previous_release}} expanded to %q, want %q", got, host.ReleasePath("1"))
	}
}

// Spec 7.3: the no-op fast path fires when "the live revision already equals the
// target *and its record is complete*". Only the revision was checked, so a
// retry after a deploy that failed post-activation reported "already deployed"
// and exited 0 — success reported for a release whose record says it failed.
func TestNoOpFastPathRequiresACompleteRecord(t *testing.T) {
	seed := func(status string) (deploy.Host, deploy.Plan) {
		t.Helper()
		host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
		ctx := context.Background()
		if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("1")+"/.dep", deploy.RunOptions{}); err != nil {
			t.Fatalf("seed release: %v", err)
		}
		record := deploy.NewReleaseForTest("1", "abc", "main")
		record.Status = status
		if err := deploy.WriteRelease(ctx, host, record); err != nil {
			t.Fatalf("seed record: %v", err)
		}
		if _, err := host.Runner().Run(ctx, "ln -s "+host.ReleasePath("1")+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
			t.Fatalf("seed symlink: %v", err)
		}
		plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
			{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "true"},
		}), nil, "local")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		return host, plan
	}

	run := func(host deploy.Host, plan deploy.Plan) deploy.Outcome {
		t.Helper()
		options := deploy.Options{Remote: "local", Publish: deploy.PublishSymlink, CommandTimeout: time.Minute}
		outcome, err := deploy.NewExecutor(host, options, io.Discard).Run(
			context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("", "abc", "main"))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return outcome
	}

	complete, plan := seed(deploy.StatusOK)
	if outcome := run(complete, plan); !outcome.AlreadyDeployed {
		t.Fatal("a complete record for the same revision must take the no-op path")
	}

	failed, plan := seed(deploy.StatusFailed)
	if outcome := run(failed, plan); outcome.AlreadyDeployed {
		t.Fatal("a failed record must not be reported as already deployed")
	}

	running, plan := seed(deploy.StatusRunning)
	if outcome := run(running, plan); outcome.AlreadyDeployed {
		t.Fatal("an unfinished record must not be reported as already deployed")
	}
}

// recordingRunner captures what the executor asked the transport to run. A test
// cannot assert the credential handoff through LocalRunner: a local command
// inherits the test process's environment, so the variable would be there whether
// or not govard passed it.
type recordingRunner struct {
	inner    deploy.Runner
	commands []string
	options  []deploy.RunOptions
}

func (r *recordingRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	r.commands = append(r.commands, command)
	r.options = append(r.options, opts)
	return r.inner.Run(ctx, command, opts)
}

// Spec 10.4: "the recipe passes credentials from COMPOSER_AUTH when the
// environment provides it (the CI and server case)". Nothing forwarded them.
//
// The handoff must keep the secret out of the target's process list (argv) and
// out of the deploy log, so it travels on the step's standard input.
func TestComposerAuthIsHandedToTheDependencyStepThroughStdin(t *testing.T) {
	const secret = `{"http-basic":{"repo.example.com":{"username":"u","password":"s3cret"}}}`
	t.Setenv("COMPOSER_AUTH", secret)

	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	runner := &recordingRunner{inner: deploy.LocalRunner{}}
	host = host.WithRunner(runner)

	recipe := deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "echo installing-dependencies"},
		{ID: deploy.TaskCompile, Stage: deploy.StageBuild, Command: "echo compiling"},
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var timeline bytes.Buffer
	options := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	if _, err := deploy.NewExecutor(host, options, &timeline).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local")); err != nil {
		t.Fatalf("run: %v", err)
	}

	index := -1
	for i := range runner.options {
		if strings.Contains(runner.commands[i], "installing-dependencies") {
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("the dependency step never ran: %v", runner.commands)
	}
	if runner.options[index].Stdin != secret {
		t.Fatalf("the dependency step's stdin = %q, want the credentials", runner.options[index].Stdin)
	}
	if !strings.Contains(runner.commands[index], `COMPOSER_AUTH="$(cat)"`) {
		t.Fatalf("the command must read the credentials from stdin, got %q", runner.commands[index])
	}
	// Every other step is left exactly as the recipe wrote it.
	for i, command := range runner.commands {
		if i != index && strings.Contains(command, "COMPOSER_AUTH") {
			t.Fatalf("step %d was wrapped too: %q", i, command)
		}
	}
	if strings.Contains(timeline.String(), secret) || strings.Contains(timeline.String(), "s3cret") {
		t.Fatalf("the deploy log must never contain the credentials:\n%s", timeline.String())
	}
}

// The prelude is what actually puts the value in the environment, so it is run
// for real — with COMPOSER_AUTH removed from the environment, which is the
// situation a remote command is in.
func TestComposerAuthPreludeSetsTheEnvironmentFromStdin(t *testing.T) {
	const secret = "credential-value"
	marker := filepath.Join(t.TempDir(), "seen")

	command := deploy.ComposerAuthCommandForTest(`printf '%s' "$COMPOSER_AUTH" > ` + marker)
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(secret)
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "COMPOSER_AUTH=") {
			env = append(env, entry)
		}
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run the prelude: %v\n%s", err, out)
	}

	seen, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(seen) != secret {
		t.Fatalf("COMPOSER_AUTH = %q, want the value carried on stdin", seen)
	}
}

func TestNoComposerAuthLeavesTheCommandAlone(t *testing.T) {
	t.Setenv("COMPOSER_AUTH", "")

	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	runner := &recordingRunner{inner: deploy.LocalRunner{}}
	host = host.WithRunner(runner)

	recipe := deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "echo installing-dependencies"},
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	options := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local")); err != nil {
		t.Fatalf("run: %v", err)
	}
	index := -1
	for i, command := range runner.commands {
		if strings.Contains(command, "installing-dependencies") {
			index = i
		}
		if strings.Contains(command, "COMPOSER_AUTH") {
			t.Fatalf("without credentials no command may be wrapped: %q", command)
		}
	}
	if index < 0 {
		t.Fatalf("the dependency step never ran: %v", runner.commands)
	}
	if runner.commands[index] != "echo installing-dependencies" {
		t.Fatalf("the command must be untouched, got %q", runner.commands[index])
	}
	if runner.options[index].Stdin != "" {
		t.Fatalf("without credentials nothing may be piped, got %q", runner.options[index].Stdin)
	}
}
