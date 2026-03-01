package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func TestDesktopSmokeOnboardingRemotesShellActionsSettings(t *testing.T) {
	home := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	desktop.ResetStateForTest()

	projectRoot := t.TempDir()
	baseConfig := strings.TrimSpace(`
project_name: smoke
framework: laravel
domain: smoke.test
stack:
  php_version: "8.3"
  node_version: "22"
  db_type: mariadb
  db_version: "10.6"
  web_root: /public
  services:
    web_server: nginx
    search: none
    cache: none
    queue: none
  features:
    xdebug: true
    varnish: false
remotes:
  staging:
    host: stage.example.com
    user: deploy
    path: /var/www/stage
    environment: staging
`) + "\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".govard.yml"), []byte(baseConfig), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	onboardMessage, err := desktop.OnboardProjectWithOptionsForPathForTest(
		projectRoot,
		"",
		"smoke-local",
		true,
		true,
		true,
		false,
	)
	if err != nil {
		t.Fatalf("onboard project with overrides: %v", err)
	}
	if !strings.Contains(strings.ToLower(onboardMessage), "added") {
		t.Fatalf("expected onboarding add message, got %q", onboardMessage)
	}

	capturedArgs := []string{}
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(root string, args []string) (string, error) {
		if root != projectRoot {
			t.Fatalf("unexpected root path %q", root)
		}
		capturedArgs = append([]string{}, args...)
		return "Sync plan generated.", nil
	})
	defer restore()

	app := desktop.NewApp()
	planMessage, _ := app.Remote.RunRemoteSyncPreset(
		projectRoot,
		"staging",
		"db",
		map[string]bool{
			"sanitize":    true,
			"excludeLogs": true,
			"compress":    false,
		},
	)
	if !strings.Contains(planMessage, "Sync plan generated.") {
		t.Fatalf("unexpected sync plan message: %q", planMessage)
	}
	for _, expectedArg := range []string{
		"sync",
		"--source",
		"staging",
		"--destination",
		"local",
		"--db",
		"--exclude",
		".env",
		"var/log/**",
		"--no-compress",
		"--plan",
	} {
		if !containsToken(capturedArgs, expectedArg) {
			t.Fatalf("expected %q in sync args: %#v", expectedArg, capturedArgs)
		}
	}

	saveShellMessage, err := app.SetShellUser("smoke", "www-data")
	if err != nil {
		t.Fatalf("set shell user: %v", err)
	}
	if !strings.Contains(strings.ToLower(saveShellMessage), "saved shell user") {
		t.Fatalf("unexpected shell save message: %q", saveShellMessage)
	}
	if got, err := app.GetShellUser("smoke"); err != nil || got != "www-data" {
		t.Fatalf("expected shell user www-data, got %q (err: %v)", got, err)
	}
	resetShellMessage, err := app.ResetShellUsers()
	if err != nil {
		t.Fatalf("reset shell users: %v", err)
	}
	if !strings.Contains(strings.ToLower(resetShellMessage), "reset") {
		t.Fatalf("unexpected shell reset message: %q", resetShellMessage)
	}

	settingsMessage, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:              "dark",
		ProxyTarget:        "smoke.test",
		PreferredBrowser:   "default",
		CodeEditor:         "code",
		DBClientPreference: "desktop",
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if settingsMessage != "Settings updated" {
		t.Fatalf("unexpected settings message: %q", settingsMessage)
	}
	settings, _ := app.Settings.GetSettings()
	if settings.ProxyTarget != "smoke.test" {
		t.Fatalf("expected normalized proxy target smoke.test, got %q", settings.ProxyTarget)
	}

	mailAction, err := app.QuickAction("open-mail-client")
	if err != nil {
		t.Fatalf("quick action mail: %v", err)
	}
	if !strings.Contains(strings.ToLower(mailAction), "mailpit") {
		t.Fatalf("expected mail quick action message, got %q", mailAction)
	}
	dbAction, err := app.QuickActionForProject("open-db-client", "smoke")
	if err != nil {
		t.Fatalf("quick action db: %v", err)
	}
	if !strings.Contains(strings.ToLower(dbAction), "opening db client") {
		t.Fatalf("expected db quick action message, got %q", dbAction)
	}
	unsupportedAction, err := app.QuickActionForProject("other", "smoke")
	if err == nil {
		t.Fatalf("expected error for unsupported action, got message: %q", unsupportedAction)
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected explicit unknown action error, got %q", err.Error())
	}
}

func containsToken(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
