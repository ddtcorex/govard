package tests

import (
	"testing"

	cmdpkg "govard/internal/cmd"
)

// Wails v3 gates dev behaviour behind !production, so the release build keeps
// the production tag on every OS and no WebKit version tag is needed.
func TestDesktopBuildTagsAreDesktopProductionOrDesktop(t *testing.T) {
	if got := cmdpkg.DesktopBuildTagsForTest(true); got != "desktop,production" {
		t.Fatalf("production build tags = %q, want %q", got, "desktop,production")
	}
	if got := cmdpkg.DesktopBuildTagsForTest(false); got != "desktop" {
		t.Fatalf("dev build tags = %q, want %q", got, "desktop")
	}
}
