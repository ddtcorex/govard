package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// A successful stop+recreate fallback must not swallow the original restart
// failure: without it there is no trace of why the primary path failed.
func TestDesktopPkgRestartFallbackKeepsOriginalError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte("project_name: fb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		switch strings.Join(args, " ") {
		case "env restart":
			return "", fmt.Errorf("restart boom")
		case "env stop":
			return "stopped", nil
		case "up --force-recreate --remove-orphans":
			return "up ok", nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.Environment.RestartEnvironment(root)
	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if !strings.Contains(message, "restart boom") {
		t.Fatalf("fallback message lost the original failure: %q", message)
	}
}
