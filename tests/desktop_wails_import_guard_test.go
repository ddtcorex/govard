package tests

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the runtime adapters may import the Wails v3 runtime. Everything else
// goes through Platform, so the untagged CLI build never links a GUI toolkit.
var wailsImportAllowlist = map[string]bool{
	"platform_wails.go":          true,
	"run_desktop.go":             true,
	"service_lifecycle_wails.go": true,
	"events_wails.go":            true,
	"tray_wails.go":              true,
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

// The migration to Wails v3 is complete when nothing imports v2 any more,
// including the files the allowlist above lets import v3.
func TestDesktopNoWailsV2ImportAnywhere(t *testing.T) {
	skipDirs := map[string]bool{"node_modules": true, "vendor": true, "build": true, "dist": true, ".git": true}
	var offenders []string
	for _, tree := range []string{"internal", "cmd", "desktop"} {
		root := filepath.Join("..", tree)
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if skipDirs[entry.Name()] {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				t.Fatalf("parse %s: %v", path, parseErr)
			}
			for _, imp := range file.Imports {
				if strings.Contains(imp.Path.Value, "wailsapp/wails/v2") {
					offenders = append(offenders, path+" imports "+imp.Path.Value)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", root, walkErr)
		}
	}
	for _, offender := range offenders {
		t.Errorf("%s; the desktop app runs on Wails v3 now", offender)
	}
}
