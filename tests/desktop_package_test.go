package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgDashboardJSONDoesNotExposeLegacyFields(t *testing.T) {
	data, err := json.Marshal(desktop.Dashboard{})
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}
	payload := string(data)
	for _, field := range []string{"onboardingReady", "onboardingChecks", "lastSync", "lastDeploy", "activity"} {
		if strings.Contains(payload, field) {
			t.Fatalf("dashboard should not expose legacy field %q: %s", field, payload)
		}
	}
}

func TestDesktopPkgGetDashboardDoesNotInjectOnboardingWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "")
	desktop.ResetStateForTest()

	dashboard := desktop.NewApp().GetDashboard()
	for _, warning := range dashboard.Warnings {
		if strings.Contains(strings.ToLower(warning), "onboarding") {
			t.Fatalf("unexpected onboarding warning in lightweight dashboard: %q", warning)
		}
	}
}

func TestDesktopPkgSettingsNormalizationViaApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	msg := app.UpdateSettings("dark", "https://govard.test/", "firefox", "code")
	if msg != "Settings updated" {
		t.Fatalf("unexpected update settings message: %s", msg)
	}

	settings := app.GetSettings()
	if settings.ProxyTarget != "govard.test" {
		t.Fatalf("expected normalized proxy target govard.test, got %s", settings.ProxyTarget)
	}
}

func TestDesktopPkgSettingsJSONDoesNotExposeRoleMode(t *testing.T) {
	data, err := json.Marshal(desktop.DesktopSettings{})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if strings.Contains(string(data), "roleMode") {
		t.Fatalf("settings should not expose roleMode: %s", string(data))
	}
}

func TestDesktopPkgLegacyPreferencesStillLoadCoreSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	desktop.ResetStateForTest()

	prefsPath := filepath.Join(home, ".govard", "desktop-preferences.json")
	if err := os.MkdirAll(filepath.Dir(prefsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]interface{}{
		"configOverrides": map[string]string{},
		"settings": map[string]string{
			"theme":            "system",
			"proxyTarget":      "govard.test",
			"preferredBrowser": "firefox",
			"roleMode":         "developer",
		},
		"lastSync": map[string]string{
			"action": "sync", "project": "demo", "status": "success",
		},
		"lastDeploy": map[string]string{
			"action": "deploy", "project": "demo", "status": "failed",
		},
		"operationHistory": []map[string]string{
			{"id": "op-a", "action": "sync", "project": "demo", "status": "success", "output": "sync ok"},
			{"id": "op-b", "action": "deploy", "project": "demo", "status": "failed", "output": "deploy failed"},
		},
		"shellUsers": map[string]string{
			"demo": "www-data",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prefsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	desktop.ResetStateForTest()
	app := desktop.NewApp()
	settings := app.GetSettings()
	if settings.ProxyTarget != "govard.test" {
		t.Fatalf("expected proxy target to load, got %q", settings.ProxyTarget)
	}
	if settings.PreferredBrowser != "firefox" {
		t.Fatalf("expected preferred browser to load, got %q", settings.PreferredBrowser)
	}
	if got := app.GetShellUser("demo"); got != "www-data" {
		t.Fatalf("expected persisted shell user, got %q", got)
	}
}
