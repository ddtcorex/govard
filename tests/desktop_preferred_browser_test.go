package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
)

func TestDesktopPreferredBrowserUsesConfiguredCommandForQuickAction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shim based test is not supported on windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	desktop.ResetStateForTest()

	logPath := filepath.Join(t.TempDir(), "preferred-browser.log")
	browserShimPath := filepath.Join(t.TempDir(), "browser-shim.sh")
	shimScript := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$1\" >> " + shellSingleQuote(logPath) + "\n"
	if err := os.WriteFile(browserShimPath, []byte(shimScript), 0o755); err != nil {
		t.Fatalf("write browser shim: %v", err)
	}

	app := desktop.NewApp()
	_, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:            "system",
		ProxyTarget:      "govard.test",
		PreferredBrowser: browserShimPath,
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}

	message, err := app.Environment.QuickAction("open-mail-client")
	if err != nil {
		t.Fatalf("open mail quick action: %v", err)
	}
	if strings.Contains(strings.ToLower(message), "open manually") {
		t.Fatalf("expected preferred browser execution, got fallback message: %q", message)
	}

	data, err := waitForFile(logPath, 2*time.Second)
	if err != nil {
		t.Fatalf("read browser shim log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	expected := "https://mail.govard.test"
	if got != expected {
		t.Fatalf("expected preferred browser to receive %q, got %q", expected, got)
	}
}

func TestDesktopPreferredBrowserFallsBackWhenCommandUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	_, err := app.Settings.UpdateSettings(desktop.DesktopSettings{
		Theme:            "system",
		ProxyTarget:      "govard.test",
		PreferredBrowser: "definitely-not-a-real-browser-command",
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}

	message, err := app.Environment.QuickAction("open-mail-client")
	if err != nil {
		t.Fatalf("open mail quick action: %v", err)
	}
	normalized := strings.ToLower(message)
	if !strings.Contains(normalized, "open manually") {
		t.Fatalf("expected fallback message with manual URL guidance, got %q", message)
	}
}

func shellSingleQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func waitForFile(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}
