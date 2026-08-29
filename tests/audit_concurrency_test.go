package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// TestAuditConcurrencyQueuesNotCancels verifies that two parallel audit runs for
// the same projectId do not cancel each other; the second waits for the first.
// This is the RED test for the queue/lock feature (example-project-large 15 cancelled).
func TestAuditConcurrencyQueuesNotCancels(t *testing.T) {
	projectID := "project-queue-test-1234"
	// Use isolated GOVARD_HOME_DIR so we do not pollute the real home.
	tmpHome := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", tmpHome)
	// Ensure the lock helpers are available; if not, test fails showing missing feature.
	lockPath := cmd.AuditLockPathForTest(projectID)
	if lockPath == "" {
		t.Fatalf("AuditLockPathForTest not implemented")
	}
	expectedSuffix := filepath.Join("audit", projectID, "lock")
	if !containsSuffix(lockPath, expectedSuffix) {
		t.Fatalf("lock path %q does not contain %q", lockPath, expectedSuffix)
	}
	// First acquire should succeed immediately.
	release1, err := cmd.AcquireAuditLockForTest(projectID)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() { _ = release1() }()

	// Second acquire should wait, not cancel. We run it in a goroutine with timeout.
	done := make(chan error, 1)
	var release2 func() error
	go func() {
		r, err := cmd.AcquireAuditLockForTest(projectID)
		release2 = r
		done <- err
	}()

	// Give the second acquire a moment to block on the lock.
	select {
	case err := <-done:
		t.Fatalf("second acquire should have waited but returned immediately with err=%v", err)
	case <-time.After(200 * time.Millisecond):
		// Still waiting - this is expected for queue behavior.
	}

	// Release first; second should now acquire.
	if err := release1(); err != nil {
		t.Fatalf("release first: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second acquire after release failed: %v", err)
		}
		if release2 == nil {
			t.Fatal("second acquire returned nil release")
		}
		defer func() { _ = release2() }()
		// Verify lock file exists while held
		if _, statErr := os.Stat(lockPath); statErr != nil {
			t.Fatalf("lock file missing while second holds lock: %v", statErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire did not acquire after first released (expected waiting, got timeout/cancelled)")
	}
}

func TestAuditLockIsPerProject(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", tmpHome)
	projectA := "project-a-0001"
	projectB := "project-b-0002"
	releaseA, err := cmd.AcquireAuditLockForTest(projectA)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	defer func() { _ = releaseA() }()
	// Different project should not block.
	done := make(chan error, 1)
	go func() {
		r, err := cmd.AcquireAuditLockForTest(projectB)
		if err == nil && r != nil {
			_ = r()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("acquire B blocked by A: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("acquire for different projectId blocked unexpectedly")
	}
}

func TestAuditConcurrencyNoCancelOnSecondWait(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", tmpHome)
	projectID := "project-concurrent-5678"
	// Simulate example-project-large 15 cancelled: second run should be queued, not cancelled.
	// Use channel barriers instead of sleep to avoid flakiness.
	release1, err := cmd.AcquireAuditLockForTest(projectID)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	firstStatus := "running"
	secondStatus := "pending"
	go func() {
		close(secondStarted)
		r, err := cmd.AcquireAuditLockForTest(projectID)
		if err != nil {
			secondStatus = "cancelled"
			close(secondDone)
			return
		}
		secondStatus = "waiting-then-passed"
		_ = r()
		close(secondDone)
	}()
	// Wait until the second goroutine has started attempting to acquire.
	<-secondStarted
	// Give the second acquire a moment to block on the lock file.
	select {
	case <-secondDone:
		t.Fatalf("second acquire should have waited but returned immediately")
	case <-time.After(50 * time.Millisecond):
	}
	_ = release1()
	firstStatus = "passed"
	// Wait for the second to acquire after the release.
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire did not acquire after first released (expected waiting, got timeout/cancelled)")
	}
	if secondStatus == "cancelled" {
		t.Fatalf("expected waiting, got cancelled")
	}
	if firstStatus == "cancelled" {
		t.Fatalf("first should not be cancelled")
	}
	if secondStatus != "waiting-then-passed" {
		t.Fatalf("second status = %q, want waiting-then-passed", secondStatus)
	}
	// Ensure GOVARD_HOME_DIR override was respected (no leakage to real home)
	if engine.GovardHomeDir() != tmpHome {
		t.Fatalf("GovardHomeDir = %q, want %q", engine.GovardHomeDir(), tmpHome)
	}
}

func containsSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
