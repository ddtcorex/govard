package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func noopSkipRun(t *testing.T, options deploy.Options) (deploy.Outcome, string, string) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := filepath.Join(t.TempDir(), "ran")
	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskWorkersPause, Stage: deploy.StagePublish, Command: "touch " + marker + "-pause"},
		{ID: deploy.TaskDBBackup, Stage: deploy.StagePublish, Command: "touch " + marker + "-backup"},
		{ID: deploy.TaskWorkersResume, Stage: deploy.StagePublish, Command: "touch " + marker + "-resume"},
	}), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	var out bytes.Buffer
	options.CommandTimeout = time.Minute
	release := deploy.NewReleaseForTest("1", "abc", "local")
	outcome, err := deploy.NewExecutor(host, options, &out).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return outcome, out.String(), marker
}

// A step the run's own options switch off must show as skipped, with the reason,
// not as a success tick that reads like work done.
func TestStepsTheOptionsSwitchOffAreSkippedWithAReason(t *testing.T) {
	outcome, text, marker := noopSkipRun(t, deploy.Options{})
	want := map[string]string{
		deploy.TaskWorkersPause:  "worker_control",
		deploy.TaskWorkersResume: "worker_control",
		deploy.TaskDBBackup:      "--db-backup",
	}
	for _, step := range outcome.Steps {
		reason, ok := want[step.ID]
		if !ok {
			continue
		}
		if step.Status != deploy.StepSkipped {
			t.Errorf("%s: status %q, want skipped", step.ID, step.Status)
		}
		if !strings.Contains(step.SkipReason, reason) {
			t.Errorf("%s: skip reason %q must name %q", step.ID, step.SkipReason, reason)
		}
		if !strings.Contains(text, step.ID) || !strings.Contains(text, reason) {
			t.Errorf("%s: timeline must show the reason %q:\n%s", step.ID, reason, text)
		}
	}
	for _, name := range []string{"-pause", "-backup", "-resume"} {
		if _, err := os.Stat(marker + name); err == nil {
			t.Errorf("a skipped step must not run its command (%s)", name)
		}
	}
}

func TestStepsTheOptionsSwitchOnStillRun(t *testing.T) {
	outcome, _, marker := noopSkipRun(t, deploy.Options{DBBackup: true, Settings: map[string]any{"worker_control": true}})
	for _, step := range outcome.Steps {
		if step.Status != deploy.StepOK {
			t.Errorf("%s: status %q, want ok", step.ID, step.Status)
		}
	}
	for _, name := range []string{"-pause", "-backup", "-resume"} {
		if _, err := os.Stat(marker + name); err != nil {
			t.Errorf("an enabled step must run (%s): %v", name, err)
		}
	}
}
