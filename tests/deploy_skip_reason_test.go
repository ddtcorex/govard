package tests

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

// A task the recipe never filled used to be skipped without a word. The reason
// names the task so the operator can tell "not implemented here" from a gate.
func TestUnimplementedRecipeTaskIsSkippedWithAReason(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskCompile, Stage: deploy.StageBuild},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "true"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	var out bytes.Buffer
	release := deploy.NewReleaseForTest("1", "abc", "local")
	outcome, err := deploy.NewExecutor(host, deploy.Options{CommandTimeout: time.Minute}, &out).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "this framework's recipe has no " + deploy.TaskCompile + " step"
	found := false
	for _, step := range outcome.Steps {
		if step.ID != deploy.TaskCompile {
			continue
		}
		found = true
		if step.Status != deploy.StepSkipped || step.SkipReason != want {
			t.Errorf("%s: status %q reason %q, want skipped with %q", step.ID, step.Status, step.SkipReason, want)
		}
	}
	if !found {
		t.Fatalf("%s is missing from the outcome: %+v", deploy.TaskCompile, outcome.Steps)
	}
	if !strings.Contains(out.String(), want) {
		t.Errorf("the timeline must show the reason:\n%s", out.String())
	}
}

func TestResumeCarriedStepsSayTheyCompletedInTheInterruptedRun(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "true"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	release := deploy.NewReleaseForTest("3", "abc", "local")
	release.Tasks = []deploy.StepRecord{{ID: deploy.TaskVendors, Status: deploy.StepOK}}
	if err := deploy.WriteRelease(context.Background(), host, release); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	var out bytes.Buffer
	outcome, err := deploy.NewExecutor(host, deploy.Options{CommandTimeout: time.Minute, Resume: true}, &out).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	const want = "completed in the interrupted run"
	for _, step := range outcome.Steps {
		if step.ID == deploy.TaskVendors && step.SkipReason != want {
			t.Errorf("carried step reason = %q, want %q", step.SkipReason, want)
		}
	}
	if !strings.Contains(out.String(), want) {
		t.Errorf("the timeline must show the reason:\n%s", out.String())
	}
}

func TestStepsBeforeFromSayTheyWereNotRunBecauseOfResume(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "true"},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "true"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	release := deploy.NewReleaseForTest("4", "abc", "local")
	var out bytes.Buffer
	outcome, err := deploy.NewExecutor(host, deploy.Options{CommandTimeout: time.Minute, From: deploy.TaskActivate}, &out).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "not run: resumed from " + deploy.TaskActivate
	found := false
	for _, step := range outcome.Steps {
		if step.ID != deploy.TaskVendors {
			continue
		}
		found = true
		if step.Status != deploy.StepSkipped || step.SkipReason != want {
			t.Errorf("%s: status %q reason %q, want skipped with %q", step.ID, step.Status, step.SkipReason, want)
		}
	}
	if !found {
		t.Fatalf("%s missing: %+v", deploy.TaskVendors, outcome.Steps)
	}
	if !strings.Contains(out.String(), want) {
		t.Errorf("the timeline must show the reason:\n%s", out.String())
	}
}
