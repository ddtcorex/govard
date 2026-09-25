package tests

import (
	"strings"
	"testing"
)

// Files behind //go:build desktop (the Wails adapter, tray, lifecycle) are
// invisible to the untagged `make vet` / `make lint`. CI must analyse them
// in a job that has the GTK4 headers their cgo dependency needs.
func TestDesktopTaggedCodeIsVettedAndLintedInCI(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	for _, want := range []string{
		"lint-desktop:",
		"go vet -tags desktop,production ./...",
		"run --build-tags desktop,production ./...",
	} {
		if !strings.Contains(makefile, want) {
			t.Errorf("Makefile must contain %q", want)
		}
	}

	ci := readRepoFile(t, ".github/workflows/ci-pipeline.yml")
	deps := strings.Index(ci, "libwebkitgtk-6.0-dev")
	lint := strings.Index(ci, "make lint-desktop")
	if lint < 0 {
		t.Fatal("CI must run make lint-desktop")
	}
	if deps < 0 || deps > lint {
		t.Error("make lint-desktop must run after the GTK4 / WebKitGTK 6.0 headers are installed")
	}
}
