package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

// Island crashes must reach the backend log, not just the devtools console:
// support cannot see a user's devtools.
func TestSystemReportFrontendErrorForTest(t *testing.T) {
	svc := desktop.NewSystemService()
	msg, err := svc.ReportFrontendError("Panel X failed: boom")
	if err != nil {
		t.Fatalf("ReportFrontendError: %v", err)
	}
	if !strings.Contains(msg, "report") {
		t.Fatalf("unexpected confirmation: %q", msg)
	}
	if _, err := svc.ReportFrontendError("   "); err == nil {
		t.Fatal("expected empty message to be refused")
	}
}
