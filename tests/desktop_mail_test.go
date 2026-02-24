package tests

import (
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgGetMailpitURLUsesDefaultProxyTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	if got := app.GetMailpitURL(); got != "https://mail.govard.test" {
		t.Fatalf("expected default mailpit URL, got %q", got)
	}
}

func TestDesktopPkgGetMailpitURLUsesCustomProxyTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	if message := app.UpdateSettings("system", "https://workspace.internal/", "", ""); message != "Settings updated" {
		t.Fatalf("unexpected settings update message: %s", message)
	}

	if got := app.GetMailpitURL(); got != "https://mail.workspace.internal" {
		t.Fatalf("expected custom mailpit URL, got %q", got)
	}
}
