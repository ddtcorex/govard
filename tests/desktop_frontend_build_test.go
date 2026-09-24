package tests

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/desktop/frontend"
)

func TestDesktopFrontendAssetsRootIsDist(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "desktop", "frontend", "dist", "index.html")); err != nil {
		t.Skip("dist/index.html missing; run `make frontend` first")
	}
	if _, err := fs.Stat(frontend.Assets, "index.html"); err != nil {
		t.Fatalf("frontend.Assets must serve index.html at its root: %v", err)
	}
	data, err := fs.ReadFile(frontend.Assets, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "assets/styles.css") {
		t.Fatal("built index.html still references the committed Tailwind output")
	}
}

func TestDesktopFrontendCSSOutputNotTracked(t *testing.T) {
	out, err := exec.Command("git", "-C", "..", "ls-files", "desktop/frontend/assets/styles.css", "desktop/frontend/yarn.lock").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("build outputs / old lockfile still tracked:\n%s", out)
	}
}
