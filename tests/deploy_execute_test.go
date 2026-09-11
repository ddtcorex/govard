package tests

import (
	"context"
	"io"
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
