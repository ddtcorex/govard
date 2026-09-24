package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both entry points must launch through desktop.Run so wails dev and
// release builds share one set of options (single-instance, background,
// close behaviour).
func TestDesktopEntryPointsShareRun(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("..", "cmd", "govard-desktop", "main.go"),
		filepath.Join("..", "desktop", "main.go"),
	} {
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
	}
}
