package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

func TestExecutorReleasesTheLockWhenAFailureHappensBeforePublish(t *testing.T) {
	host, plan := executorForTest(t, []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "exit 4"},
	})
	executor := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, io.Discard)
	outcome, err := executor.Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
	if err == nil {
		t.Fatal("want an error")
	}
	if outcome.LockHeld {
		t.Fatal("a prepare-stage failure must release the lock: nothing live has changed yet")
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
