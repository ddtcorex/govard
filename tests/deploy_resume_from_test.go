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

func resumeFromPlan(t *testing.T, host deploy.Host, marks map[string]string) deploy.Plan {
	t.Helper()
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskCode, Stage: deploy.StagePrepare, Command: "touch " + marks["code"]},
		{ID: deploy.TaskWritable, Stage: deploy.StagePrepare, Command: "touch " + marks["writable"]},
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "touch " + marks["activate"]},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	return plan
}

func resumeFromRecord(t *testing.T, host deploy.Host) *deploy.Release {
	t.Helper()
	release := deploy.NewReleaseForTest("3", "abc", "local")
	release.Tasks = []deploy.StepRecord{
		{ID: deploy.TaskCode, Status: deploy.StepOK},
		{ID: deploy.TaskWritable, Status: deploy.StepOK},
	}
	if err := deploy.WriteRelease(context.Background(), host, release); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	return release
}

// `--from` under `--resume` names the first step to run again: steps the record
// marks ok at or after it are repeated, earlier ones stay carried over.
func TestResumeWithFromRerunsRecordedStepsFromThatTask(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	dir := t.TempDir()
	marks := map[string]string{
		"code":     filepath.Join(dir, "code"),
		"writable": filepath.Join(dir, "writable"),
		"activate": filepath.Join(dir, "activate"),
	}
	plan := resumeFromPlan(t, host, marks)
	release := resumeFromRecord(t, host)

	options := deploy.Options{CommandTimeout: time.Minute, Resume: true, From: deploy.TaskWritable}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("resume --from: %v", err)
	}
	if _, err := os.Stat(marks["writable"]); err != nil {
		t.Fatalf("--from must re-run a step the record marks ok: %v", err)
	}
	if _, err := os.Stat(marks["activate"]); err != nil {
		t.Fatalf("steps after --from must run: %v", err)
	}
	if _, err := os.Stat(marks["code"]); err == nil {
		t.Fatal("steps before --from stay carried over from the earlier run")
	}
}

// Without --from a resume still skips what the record marks ok.
func TestResumeWithoutFromStillSkipsCompletedSteps(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	dir := t.TempDir()
	marks := map[string]string{
		"code":     filepath.Join(dir, "code"),
		"writable": filepath.Join(dir, "writable"),
		"activate": filepath.Join(dir, "activate"),
	}
	plan := resumeFromPlan(t, host, marks)
	release := resumeFromRecord(t, host)

	options := deploy.Options{CommandTimeout: time.Minute, Resume: true}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, err := os.Stat(marks["writable"]); err == nil {
		t.Fatal("a plain resume must not repeat a recorded step")
	}
	if _, err := os.Stat(marks["activate"]); err != nil {
		t.Fatalf("the unfinished step must run: %v", err)
	}
}

// deploy:shared is idempotent and is what links a shared file seeded after the
// first run failed, so a resume repeats it even though the record says ok.
func TestResumeRelinksSharedEntries(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	release := deploy.NewReleaseForTest("3", "abc", "local")
	release.Path = host.ReleasePath("3")
	if err := os.MkdirAll(release.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	release.Tasks = []deploy.StepRecord{{ID: deploy.TaskShared, Status: deploy.StepOK}}
	if err := deploy.WriteRelease(ctx, host, release); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	// Seeded after the first run, as the operator does.
	if err := os.MkdirAll(host.SharedPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host.SharedPath(), ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskShared, Stage: deploy.StagePrepare, Core: deploy.CoreShared},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	options := deploy.Options{CommandTimeout: time.Minute, Resume: true, Settings: map[string]any{"shared_files": []string{".env"}}}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(release.Path, ".env")); err != nil || target != filepath.Join(host.SharedPath(), ".env") {
		t.Fatalf("the seeded shared file must be linked on resume, got %q, %v", target, err)
	}
}
