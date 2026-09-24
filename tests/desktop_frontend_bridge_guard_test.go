package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the bridge and event modules may touch the Wails globals.
// Plan B empties this allowlist once generated bindings replace window.go.
var frontendGlobalAllowlist = map[string]bool{
	"services/bridge.js":  true,
	"services/events.js":  true,
	"types/wails-v2.d.ts": true,
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
			case "node_modules", "dist", "wailsjs":
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(rel)
		if ext != ".js" && ext != ".ts" && ext != ".html" {
			return nil
		}
		if frontendGlobalAllowlist[rel] || strings.HasSuffix(rel, ".config.js") {
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
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
