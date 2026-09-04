package cmd

import (
	"testing"
	"time"

	"govard/internal/engine"
)

// TestConfirmActionDryRunDoesNotBlock verifies the core deadlock fix from issue #219:
// under --dry-run, confirmAction must return without reading stdin (which deadlocked
// the runtime via the pterm interactive confirm when no TTY was present).
func TestConfirmActionDryRunDoesNotBlock(t *testing.T) {
	// Simulate non-interactive environment: no TTY and no GOVARD_ASSUME_YES.
	t.Setenv("GOVARD_ASSUME_YES", "false")
	oldDryRun := doctorDryRun
	doctorDryRun = true
	defer func() { doctorDryRun = oldDryRun }()

	done := make(chan bool, 1)
	go func() {
		// Should return promptly, not block forever on stdin.
		_ = confirmAction("Do you want to automatically tune the framework runtime profile now?")
		done <- true
	}()

	select {
	case <-done:
		// success: returned without deadlock
	case <-time.After(3 * time.Second):
		t.Fatal("confirmAction deadlocked under --dry-run (issue #219)")
	}
}

// TestPullRuntimeImagesDryRunShortCircuits verifies that pullRuntimeImages does not
// prompt or pull images when --dry-run is set (it previously had no dry-run guard).
func TestPullRuntimeImagesDryRunShortCircuits(t *testing.T) {
	oldDryRun := doctorDryRun
	doctorDryRun = true
	defer func() { doctorDryRun = oldDryRun }()

	// Ensure a deterministic "missing images" path never runs by confirming the
	// handler returns before reaching docker inspection.
	result := pullRuntimeImages(engine.DoctorCheck{ID: "project.runtime.images", Title: "Runtime images"})
	if result.Status == DoctorFixStatusFailed {
		t.Fatalf("dry-run pull should not fail: %s", result.Message)
	}
	if !containsDryRun(result.Message) {
		t.Fatalf("expected [dry-run] message, got %q (actions=%v)", result.Message, result.Actions)
	}
}

func containsDryRun(s string) bool {
	return len(s) >= 9 && (s[:9] == "[dry-run]" || (len(s) > 9 && containsSub(s, "[dry-run]")))
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
