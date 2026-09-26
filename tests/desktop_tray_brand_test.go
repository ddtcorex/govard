package tests

import (
	"testing"

	"govard/internal/desktop"
)

type recordingTray struct{ label, tooltip string }

func (r *recordingTray) SetLabel(label string)     { r.label = label }
func (r *recordingTray) SetTooltip(tooltip string) { r.tooltip = tooltip }

// On Linux, Wails v3's SetTooltip is a no-op and the StatusNotifierItem
// Title defaults to "Wails"; SetLabel is what sets the Title, so the tray
// must call both to read "Govard" on every platform — the name the launcher
// and the window title use.
func TestDesktopTrayIsBrandedThroughLabelAndTooltip(t *testing.T) {
	tray := &recordingTray{}
	desktop.BrandTrayForTest(tray)
	if tray.label != "Govard" {
		t.Fatalf("label = %q, want %q (the SNI Title on Linux)", tray.label, "Govard")
	}
	if tray.tooltip != "Govard" {
		t.Fatalf("tooltip = %q, want %q", tray.tooltip, "Govard")
	}
}
