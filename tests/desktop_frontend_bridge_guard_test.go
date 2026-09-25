package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The frontend must reach the Go side through services/bridge.js and subscribe
// through services/events.js; no other module may touch the runtime or the
// generated bindings. Both modules wrap the runtime, so this allowlist is
// empty now that window.go is gone.
//
// This is a substring scan, not an AST check: it catches the literal forms
// below and would miss an equivalent spelled another way.
var frontendRuntimeAllowlist = map[string]bool{
	"services/bridge.js": true,
	"services/events.js": true,
}

// Files that legitimately mention the bindings path or the runtime package.
var frontendImportAllowlist = map[string]bool{
	"services/bridge.js": true,
	"services/events.js": true,
}

func TestDesktopFrontendUsesBridgeOnly(t *testing.T) {
	root := filepath.Join("..", "desktop", "frontend")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case "node_modules", "dist", "wailsjs", "bindings":
				return filepath.SkipDir
			}
			return nil
		}
		// .tsx/.jsx are scanned too: islands and shadcn components are the files
		// most likely to reach for the runtime directly, and an extension the
		// guard skips is an extension the drift hides in.
		switch filepath.Ext(rel) {
		case ".js", ".ts", ".jsx", ".tsx", ".html":
		default:
			return nil
		}
		if frontendRuntimeAllowlist[rel] || strings.HasSuffix(rel, ".config.js") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		for _, banned := range []string{"window.go", "window.runtime", "desktopBridge.runtime"} {
			if strings.Contains(src, banned) {
				t.Errorf("%s uses %s; go through services/bridge.js or services/events.js", rel, banned)
			}
		}
		if frontendImportAllowlist[rel] {
			return nil
		}
		for _, banned := range []string{"@wailsio/runtime", "bindings/"} {
			if strings.Contains(src, banned) {
				t.Errorf("%s imports %s; go through services/bridge.js or services/events.js", rel, banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
