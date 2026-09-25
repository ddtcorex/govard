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
	// The macOS package is CLI-only until the cgo .app build returns (Spec 3):
	// it has no desktop binary to feed, so it needs no frontend build, and it
	// must not grow one back without the build step.
	macos := readRepoFile(t, "scripts/build-macos-pkg.sh")
	if strings.Contains(macos, "govard-desktop") {
		t.Error("scripts/build-macos-pkg.sh must stay CLI-only until Spec 3")
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

// The desktop toolchain is pnpm. These are the pages a contributor reads to set
// one up; docs/reference/cli-commands.md is deliberately absent because it
// documents `govard tool yarn`, which is unrelated to the frontend toolchain.
func TestDesktopDocsNoLongerMentionYarn(t *testing.T) {
	for _, rel := range []string{
		"README.md",
		"desktop/README.md",
		"desktop/frontend/README.md",
		"docs/getting-started/installation.md",
		"docs/vi/getting-started/installation.md",
		"docs/workflows/desktop-app.md",
		"docs/vi/workflows/desktop-app.md",
	} {
		if strings.Contains(strings.ToLower(readRepoFile(t, rel)), "yarn") {
			t.Errorf("%s still mentions yarn", rel)
		}
	}
}

// The preview unit tests live one directory down, and a shell glob does not
// recurse: without the second glob they never run in CI and rot silently.
func TestMakeTestFrontendRunsPreviewTests(t *testing.T) {
	target := readRepoFile(t, "Makefile")
	if !strings.Contains(target, "node --test tests/frontend/*.test.mjs tests/frontend/preview/*.test.mjs") {
		t.Error("make test-frontend must glob tests/frontend/preview/*.test.mjs too")
	}
}

// The raw-CDP behaviour tier only protects the islands if CI runs it.
func TestCIRunsFrontendBehaviourTests(t *testing.T) {
	ci := readRepoFile(t, ".github/workflows/ci-pipeline.yml")
	if !strings.Contains(ci, "make test-frontend-behaviour") {
		t.Fatal("CI must run make test-frontend-behaviour (spec: the behaviour tier runs after test-frontend)")
	}
}
