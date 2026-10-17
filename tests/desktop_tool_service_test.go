package tests

import (
	"testing"

	"govard/internal/desktop"
)

// The ToolService methods are thin Wails-bound wrappers over the package
// functions Tasks 1-3 tested: they must construct, surface resolution errors
// (never a misleading empty list), and report cancel of an unknown op as
// false without an error.
func TestToolServiceListPresetsSurfacesResolutionError(t *testing.T) {
	svc := desktop.NewToolService()
	if svc == nil {
		t.Fatal("NewToolService returned nil")
	}
	if _, err := svc.ListToolPresets(""); err == nil {
		t.Fatal("expected an error for an empty project, got nil")
	}
	if _, err := svc.ListToolPresets("definitely-not-a-project"); err == nil {
		t.Fatal("expected an error for an unresolvable project, got nil")
	}
}

func TestToolServiceCancelUnknownOpReportsFalse(t *testing.T) {
	desktop.ResetOperationRegistryForTest()
	svc := desktop.NewToolService()
	cancelled, err := svc.CancelToolRun("tool:definitely-not-running")
	if err != nil {
		t.Fatalf("cancel of an unknown op must not error: %v", err)
	}
	if cancelled {
		t.Fatal("cancel of an unknown op must report false")
	}
}

func TestToolServiceHistorySurfacesResolutionError(t *testing.T) {
	svc := desktop.NewToolService()
	if _, err := svc.GetToolHistory(""); err == nil {
		t.Fatal("expected an error for an empty project, got nil")
	}
}

func TestToolServiceRunRejectsUnknownPreset(t *testing.T) {
	svc := desktop.NewToolService()
	if _, err := svc.RunTool("sample-project", "no-such-preset", nil); err == nil {
		t.Fatal("expected an error for an unknown preset, got nil")
	}
}
