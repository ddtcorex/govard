package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The preview seam is dev-only. A build that accidentally adds preview.html to
// vite.config.js's build input, or an island imported from main.js, would ship
// dev-only code with no visible symptom (vite build exits 0 either way).
func TestDesktopFrontendPreviewNotEmbedded(t *testing.T) {
	distDir := filepath.Join("..", "desktop", "frontend", "dist")
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		t.Skip("dist/index.html missing; run `make frontend` first")
	}
	if _, err := os.Stat(filepath.Join(distDir, "preview.html")); err == nil {
		t.Fatal("dist/preview.html exists; the preview seam must never ship")
	}
	if _, err := os.Stat(filepath.Join(distDir, "preview")); err == nil {
		t.Fatal("dist/preview/ exists; the preview seam must never ship")
	}
	// The record plugin injects a script tag into index.html in dev only
	// (`apply: "serve"` plus a GOVARD_PREVIEW_RECORD check). A build that carried
	// that tag would embed a dev-only entry point in the shipped binary, and the
	// tag's own file would not even be in dist/, so the window would fail on a
	// 404 with no message.
	index := readRepoFile(t, "desktop/frontend/dist/index.html")
	for _, banned := range []string{"preview/bootstrap.js", "record-bootstrap", "__preview/record"} {
		if strings.Contains(index, banned) {
			t.Errorf("dist/index.html references %q; dev-only preview code must never ship", banned)
		}
	}
	// Dev-only artifacts must be unreachable from index.html's module graph. Only
	// genuinely dev-only strings belong here. A migrated island is production code
	// and ships in this bundle, and so do the preview-only hatches main.js reads
	// (`__govardPreviewMetricsIntervalMs`, `__govardPreviewExposeUpdatePrompt`):
	// that is the shipped design from the metrics and update-prompt migrations, not
	// a leak, so neither is a marker. The demo island, its container id and its
	// global are the things nothing in index.html's graph may mention. The bundle is
	// content-hashed, so this checks markers rather than file names.
	assets, err := filepath.Glob(filepath.Join(distDir, "assets", "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		data, err := os.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"react-demo-island-root", "__govardDemoIsland"} {
			if strings.Contains(string(data), marker) {
				t.Errorf("%s carries dev-only marker %q; it must not be reachable from index.html", filepath.Base(asset), marker)
			}
		}
	}
}

// The same rule without needing a build, so it also runs where dist/ is absent
// (CI's fast-tests job builds nothing). This is the check that would have caught
// a dev-only island being imported from main.js in the first place.
func TestDesktopMainJSReachesNoDevOnlyCode(t *testing.T) {
	src := readRepoFile(t, "desktop/frontend/main.js")
	for _, banned := range []string{"DemoIsland", "react-demo-island-root", "__govardDemoIsland"} {
		if strings.Contains(src, banned) {
			t.Errorf("main.js references %q; dev-only preview code must not be reachable from main.js", banned)
		}
	}
}
