package tests

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the platform adapter and the launch wiring may import Wails.
// Plan B (Wails v3) updates this allowlist; keep it short.
var wailsImportAllowlist = map[string]bool{
	"platform_wails.go":      true,
	"run_desktop.go":         true,
	"launch.go":              true,
	"test_helpers_launch.go": true,
}

func TestDesktopOnlyAdaptersImportWails(t *testing.T) {
	dir := filepath.Join("..", "internal", "desktop")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || wailsImportAllowlist[name] {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			if strings.Contains(imp.Path.Value, "github.com/wailsapp/wails") {
				t.Errorf("%s imports %s; route runtime calls through Platform instead", name, imp.Path.Value)
			}
		}
	}
}
