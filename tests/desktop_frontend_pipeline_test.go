package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// Every path that builds govard-desktop must build the Vite frontend first,
// or the binary embeds an empty dist/ and shows a blank window.
func TestDesktopBuildPathsBuildFrontendFirst(t *testing.T) {
	if !strings.Contains(readRepoFile(t, ".goreleaser.yml"), "make frontend") {
		t.Error(".goreleaser.yml must run `make frontend` in before.hooks")
	}
	macos := readRepoFile(t, "scripts/build-macos-pkg.sh")
	if !strings.Contains(macos, "make frontend") {
		t.Error("scripts/build-macos-pkg.sh must run `make frontend` before building govard-desktop")
	}
	install := readRepoFile(t, "install.sh")
	frontendIdx := strings.Index(install, "pnpm build")
	desktopIdx := strings.Index(install, "cmd/govard-desktop/main.go")
	if frontendIdx < 0 || desktopIdx < 0 || frontendIdx > desktopIdx {
		t.Error("install.sh must run `pnpm build` for the frontend before building cmd/govard-desktop")
	}
	if !strings.Contains(readRepoFile(t, "Makefile"), "pnpm --dir desktop/frontend typecheck") {
		t.Error("make test-frontend must run the frontend typecheck")
	}
}

func TestDesktopDocsNoLongerMentionYarn(t *testing.T) {
	for _, rel := range []string{"desktop/README.md", "desktop/frontend/README.md", "docs/workflows/desktop-app.md", "docs/vi/workflows/desktop-app.md"} {
		if strings.Contains(strings.ToLower(readRepoFile(t, rel)), "yarn") {
			t.Errorf("%s still mentions yarn", rel)
		}
	}
}
