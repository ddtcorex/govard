package tests

import (
	"testing"

	"govard/internal/desktop"
)

// The tray must offer Stop (not Start-and-recreate) for anything still
// running: degraded, warning and syncing count alongside running.
func TestDesktopPkgTrayRunningStatus(t *testing.T) {
	running := []string{"running", "degraded", "warning", "syncing"}
	for _, status := range running {
		if !desktop.IsTrayRunningStatusForTest(status) {
			t.Errorf("expected %q to count as running in the tray", status)
		}
	}
	for _, status := range []string{"stopped", "", "bogus"} {
		if desktop.IsTrayRunningStatusForTest(status) {
			t.Errorf("expected %q to count as stopped in the tray", status)
		}
	}
}

// The header counter and the tray share one running rule: every status the
// tray treats as running must count as active, or the menu and the counter
// disagree about the same project.
func TestDesktopPkgActiveCounterCoversTrayRunning(t *testing.T) {
	for _, status := range []string{"running", "degraded", "warning", "syncing"} {
		if !desktop.IsTrayRunningStatusForTest(status) {
			t.Fatalf("parity test setup wrong: tray does not run %q", status)
		}
		if !desktop.IsActiveEnvironmentStatusForTest(status) {
			t.Errorf("counter misses tray-running status %q", status)
		}
	}
}
