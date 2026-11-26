package cmd

import (
	"errors"
	"strings"
	"testing"
)

// TestFallbackSyncPlanSummaryPrintsNoCommand pins the reason the display-only
// rsync builder was deleted: the degraded summary must not show a command it
// cannot verify, and must say why.
func TestFallbackSyncPlanSummaryPrintsNoCommand(t *testing.T) {
	lines := buildFallbackSyncPlanSummary("staging", "local", SyncExecutionOptions{
		Files:  true,
		Delete: true,
	}, errors.New("remote 'staging' has no path"))

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "rsync -a") || strings.Contains(joined, "--exclude") {
		t.Fatalf("the fallback summary prints an rsync command it cannot build:\n%s", joined)
	}
	if !strings.Contains(joined, "needs resolved endpoints") {
		t.Fatalf("the fallback summary does not say why there is no command:\n%s", joined)
	}
	if !strings.Contains(joined, "Delete Mode: Enabled") {
		t.Fatalf("the fallback summary lost the scope and delete facts:\n%s", joined)
	}
}
