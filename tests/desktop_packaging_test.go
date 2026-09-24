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

// The macOS package is CLI-only, so its release job must not download or
// checksum a desktop archive that build-macos-pkg.sh no longer produces, and it
// no longer needs Node or pnpm. A tag-only workflow never runs in PR CI, so a
// stale glob there only fails on a real release.
func TestDesktopMacOSReleaseJobIsCLIOnly(t *testing.T) {
	release := readRepoFile(t, ".github/workflows/release.yml")
	for _, gone := range []string{"dist/*.tar.gz", "*.tar.gz >> checksums.txt"} {
		if strings.Contains(release, gone) {
			t.Errorf(".github/workflows/release.yml still references %q; the macOS package is CLI-only", gone)
		}
	}
	if !strings.Contains(release, "dist/*.pkg") {
		t.Error(".github/workflows/release.yml must upload the macOS .pkg")
	}
	if !strings.Contains(release, "*.pkg >> checksums.txt") {
		t.Error("the macOS checksum step must cover the .pkg it actually ships")
	}
	// scripts/build-macos-pkg.sh builds the CLI only, so the job that runs it
	// needs neither Node nor pnpm.
	macosJob := release[strings.Index(release, "macos-pkg:"):]
	if end := strings.Index(macosJob, "npm-publish:"); end > 0 {
		macosJob = macosJob[:end]
	}
	for _, gone := range []string{"actions/setup-node", "name: Install pnpm"} {
		if strings.Contains(macosJob, gone) {
			t.Errorf("the macOS package job still sets up %q although the CLI build does not need it", gone)
		}
	}
}

// The desktop toolchain is Go modules, pnpm and Vite. Instructions to install
// the v2 Wails CLI, or to bump wails.json, send a contributor somewhere that no
// longer exists.
func TestDesktopDocsDoNotReferenceWailsV2(t *testing.T) {
	for _, rel := range []string{
		"README.md",
		"docs/getting-started/installation.md",
		"docs/vi/getting-started/installation.md",
		"docs/developer/contributing.md",
		"docs/vi/developer/contributing.md",
	} {
		src := readRepoFile(t, rel)
		for _, gone := range []string{"wails/v2", "wails.json"} {
			if strings.Contains(src, gone) {
				t.Errorf("%s still references %s; the desktop app is a Go module tool on Wails v3", rel, gone)
			}
		}
	}
}
