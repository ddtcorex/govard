package tests

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/deploy"
)

func seedUnlockTarget(t *testing.T, tasks []deploy.StepRecord) deploy.Host {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("4", "abc", "main")
	release.Status = deploy.StatusFailed
	release.Tasks = tasks
	if err := deploy.WriteRelease(context.Background(), host, release); err != nil {
		t.Fatalf("seed the record: %v", err)
	}
	if err := os.MkdirAll(host.LockPath(), 0o755); err != nil {
		t.Fatalf("seed the lock: %v", err)
	}
	return host
}

// `deploy unlock --force` clears only the lock. When the interrupted release had
// switched maintenance mode on and never switched it off, the site keeps
// answering 503, and the output has to say so and name the step that ends it.
func TestUnlockHintsAtMaintenanceTheInterruptedReleaseLeftOn(t *testing.T) {
	host := seedUnlockTarget(t, []deploy.StepRecord{
		{ID: deploy.TaskMaintenanceEnable, Status: deploy.StepOK},
		{ID: deploy.TaskDBMigrate, Status: deploy.StepFailed},
	})
	var out bytes.Buffer
	if err := cmd.UnlockForTest(context.Background(), host, "sandbox", deploy.Options{LockStaleAfter: time.Hour}, true, &out); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	text := out.String()
	for _, want := range []string{deploy.TaskMaintenanceDisable, "maintenance", "--from"} {
		if !strings.Contains(text, want) {
			t.Errorf("the hint must mention %q, got:\n%s", want, text)
		}
	}
	if _, err := os.Stat(host.LockPath()); err == nil {
		t.Error("the lock must still be released")
	}
}

func TestUnlockStaysQuietWhenMaintenanceWasNeverEnabledOrWasDisabled(t *testing.T) {
	for name, tasks := range map[string][]deploy.StepRecord{
		"never enabled": {{ID: deploy.TaskDBMigrate, Status: deploy.StepFailed}},
		"disabled": {
			{ID: deploy.TaskMaintenanceEnable, Status: deploy.StepOK},
			{ID: deploy.TaskMaintenanceDisable, Status: deploy.StepOK},
		},
		"skipped window": {{ID: deploy.TaskMaintenanceEnable, Status: deploy.StepSkipped}},
	} {
		host := seedUnlockTarget(t, tasks)
		var out bytes.Buffer
		if err := cmd.UnlockForTest(context.Background(), host, "sandbox", deploy.Options{LockStaleAfter: time.Hour}, true, &out); err != nil {
			t.Fatalf("%s: unlock: %v", name, err)
		}
		if strings.Contains(out.String(), "maintenance") {
			t.Errorf("%s: no maintenance hint expected, got:\n%s", name, out.String())
		}
	}
}
