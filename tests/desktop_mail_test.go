package tests

import (
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgGetMailpitURLUsesDefaultProxyTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	if got := app.Settings.GetMailpitURL(); got != "https://mail.govard.test" {
		t.Fatalf("expected default mailpit URL, got %q", got)
	}
}

func TestDesktopPkgGetMailpitURLUsesCustomProxyTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	resp, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:              "dark",
		ProxyTarget:        "localhost:8025",
		PreferredBrowser:   "chrome",
		CodeEditor:         "vscode",
		DBClientPreference: "sequel",
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if resp != "Settings updated" {
		t.Fatalf("unexpected settings update message: %s", resp)
	}

	url := app.Settings.GetMailpitURL()
	expected := "https://mail.localhost:8025"
	if url != expected {
		t.Fatalf("expected custom mailpit URL, got %q", url)
	}
}
