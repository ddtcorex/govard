package tests

import (
	"os"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgGetDashboard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	data, err := app.Environment.GetDashboard()
	if err != nil {
		t.Fatalf("GetDashboard failed: %v", err)
	}

	if data.ActiveSummary == "" {
		t.Fatalf("expected some summary message, got empty string")
	}
}

func TestDesktopPkgSettingsWorkflow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	msg, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:              "dark",
		ProxyTarget:        "govard.test",
		PreferredBrowser:   "firefox",
		CodeEditor:         "vscode",
		DBClientPreference: "desktop",
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if msg != "Settings updated" {
		t.Fatalf("unexpected update settings message: %s", msg)
	}

	settings, err := app.Settings.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.ProxyTarget != "govard.test" {
		t.Fatalf("expected normalized proxy target govard.test, got %s", settings.ProxyTarget)
	}
}

func TestDesktopPkgOnboardProjectPickDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	path, err := app.Onboarding.PickProjectDirectory()
	if err != nil {
		if err.Error() == "desktop runtime not available" {
			t.Skip("skipping PickProjectDirectory test: desktop runtime not available")
			return
		}
		t.Fatalf("PickProjectDirectory failed: %v", err)
	}
	// Wails PickDirectory returns empty string in headless/no-dialog environments
	if path != "" {
		t.Fatalf("expected empty path in test environment, got %s", path)
	}
}

func TestDesktopPkgUpdateSettingsNormalization(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	_, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:              "INVALID",
		ProxyTarget:        "  http://LOCAL.test/  ",
		PreferredBrowser:   "  chrome  ",
		CodeEditor:         " code  ",
		DBClientPreference: " PMA ",
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	settings, err := app.Settings.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.Theme != "system" {
		t.Fatalf("expected invalid theme to normalize to system, got %s", settings.Theme)
	}
	if settings.ProxyTarget != "LOCAL.test" {
		t.Fatalf("expected normalized proxy target LOCAL.test, got %s", settings.ProxyTarget)
	}
	if settings.PreferredBrowser != "chrome" {
		t.Fatalf("expected trimmed preferred browser, got %q", settings.PreferredBrowser)
	}
	if settings.CodeEditor != "code" {
		t.Fatalf("expected trimmed code editor, got %q", settings.CodeEditor)
	}
	if settings.DBClientPreference != "pma" {
		t.Fatalf("expected normalized db client preference, got %q", settings.DBClientPreference)
	}
}

func TestDesktopPkgResetSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	// First change something
	_, _ = app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:       "dark",
		ProxyTarget: "custom.test",
	})

	msg, err := app.Settings.ResetSettings()
	if err != nil {
		t.Fatalf("ResetSettings failed: %v", err)
	}
	if msg != "Settings reset" {
		t.Fatalf("unexpected reset settings message: %s", msg)
	}

	settings, err := app.Settings.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.ProxyTarget != "govard.test" {
		t.Fatalf("expected reset proxy target govard.test, got %s", settings.ProxyTarget)
	}
}

func TestDesktopPkgGetUserInfo(t *testing.T) {
	app := desktop.NewApp()
	user, err := app.GetUserInfo()
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}
	if user.Username != os.Getenv("USER") && user.Username != "unknown" {
		t.Fatalf("expected current user, got %+v", user)
	}
}
