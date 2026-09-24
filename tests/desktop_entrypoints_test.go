package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The desktop binary is the only entry point, and it must launch through
// desktop.Run so the release build and `govard desktop --dev` share one set of
// options (single instance, background, close behaviour).
func TestDesktopEntryPointsShareRun(t *testing.T) {
	rel := filepath.Join("..", "cmd", "govard-desktop", "main.go")
	data, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	src := string(data)
	if !strings.Contains(src, "desktop.Run(") {
		t.Errorf("%s must call desktop.Run", rel)
	}
	if strings.Contains(src, "wails.Run(") || strings.Contains(src, "options.App{") {
		t.Errorf("%s must not build Wails options itself", rel)
	}

	// desktop/main.go was the Wails v2 CLI's entry point; v3 needs neither it
	// nor wails.json, and a leftover would build a second, unbranded binary.
	if _, err := os.Stat(filepath.Join("..", "desktop", "main.go")); err == nil {
		t.Error("desktop/main.go must not exist: cmd/govard-desktop is the only entry point")
	}
	if _, err := os.Stat(filepath.Join("..", "desktop", "wails.json")); err == nil {
		t.Error("desktop/wails.json must not exist: nothing reads it on Wails v3")
	}
}
