package tests

import (
	"testing"

	"govard/internal/desktop"
)

// The header counter must agree with the tray and the hero: degraded,
// warning and syncing are still running, only stopped is not.
func TestDesktopPkgIsActiveEnvironmentStatus(t *testing.T) {
	active := []string{"running", "degraded", "warning", "syncing", "healthy", "up"}
	for _, status := range active {
		if !desktop.IsActiveEnvironmentStatusForTest(status) {
			t.Errorf("expected %q to count as active", status)
		}
	}
	inactive := []string{"stopped", "", "bogus"}
	for _, status := range inactive {
		if desktop.IsActiveEnvironmentStatusForTest(status) {
			t.Errorf("expected %q to count as inactive", status)
		}
	}
}
