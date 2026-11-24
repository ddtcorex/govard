package tests

import (
	"testing"

	"govard/internal/desktop"
)

type recordingTray struct{ label, tooltip string }

func (r *recordingTray) SetLabel(label string)     { r.label = label }
func (r *recordingTray) SetTooltip(tooltip string) { r.tooltip = tooltip }

// On Linux the two calls write different StatusNotifierItem properties: SetLabel
// sets Title (measured on v3.1.0: with only SetTooltip the Title reads "Wails"),
// and SetTooltip sets the line shown under it. The tray must call both to read
// "Govard" on every platform, the name the launcher and the window title use.
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
