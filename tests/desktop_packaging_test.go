package tests

import (
	"strings"
	"testing"
)

// The desktop app runs on GTK 4 and WebKitGTK 6.0, which Ubuntu 24.04+ and
// Debian 13+ ship. Every build and packaging path must agree, and no path may
// still ask for the GTK 3 / WebKitGTK 4.1 stack the v2 build needed.
func TestDesktopPackagingTargetsGTK4(t *testing.T) {
	goreleaser := readRepoFile(t, ".goreleaser.yml")
	for _, want := range []string{"libwebkitgtk-6.0-4", "libgtk-4-1"} {
		if !strings.Contains(goreleaser, want) {
			t.Errorf(".goreleaser.yml must depend on %s", want)
		}
	}
	for _, rel := range []string{".goreleaser.yml", "install.sh", ".github/workflows/ci-pipeline.yml", ".github/workflows/release.yml", "scripts/build-macos-pkg.sh", "internal/cmd/desktop.go"} {
		src := readRepoFile(t, rel)
		for _, gone := range []string{"webkit2gtk-4.1", "webkit2_41", "libgtk-3-dev", "wails.json"} {
			if strings.Contains(src, gone) {
				t.Errorf("%s still references %s", rel, gone)
			}
		}
	}
	for _, rel := range []string{".github/workflows/ci-pipeline.yml", ".github/workflows/release.yml"} {
		if !strings.Contains(readRepoFile(t, rel), "libwebkitgtk-6.0-dev") {
			t.Errorf("%s must install libwebkitgtk-6.0-dev", rel)
		}
	}
	if !strings.Contains(readRepoFile(t, ".github/workflows/ci-pipeline.yml"), "make bindings-check") {
		t.Error("CI must run make bindings-check")
	}
	if strings.Contains(readRepoFile(t, "scripts/build-macos-pkg.sh"), "govard-desktop") {
		t.Error("the macOS pkg is CLI-only until Spec 3")
	}
}
