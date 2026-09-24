package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The desktop binary stub still exists for the untagged build: `go run
// ./cmd/govard-desktop` must explain that the GUI needs the desktop tag rather
// than fail to compile.
func TestEntrypointDesktopStub(t *testing.T) {
	cmd := exec.Command("go", "run", "../cmd/govard-desktop")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run ../cmd/govard-desktop failed: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "not built yet") {
		t.Fatalf("expected not-built message, got %q", string(output))
	}
}

// desktop/ is no longer a package: the Wails v2 CLI built the app from there,
// and Wails v3 builds cmd/govard-desktop instead.
func TestDesktopDirectoryHasNoGoPackage(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "desktop"))
	if err != nil {
		t.Fatalf("read desktop/: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("desktop/%s: the desktop entry point lives in cmd/govard-desktop", entry.Name())
		}
	}
}
