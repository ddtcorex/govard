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
}
