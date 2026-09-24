package tests

import (
	"testing"

	"govard/internal/cmd"
)

// Releases publish a govard-desktop archive for Linux only until the macOS and
// Windows desktop builds return (Spec 3). On a Mac that still has a v2 desktop
// binary installed, self-update must skip it rather than fail on a 404.
func TestSelfUpdateSkipsDesktopWhereReleaseShipsNone(t *testing.T) {
	cases := map[string]bool{"linux": true, "darwin": false, "windows": false}
	for goos, want := range cases {
		if got := cmd.DesktopReleaseShipsForTest(goos); got != want {
			t.Errorf("desktopReleaseShipsFor(%q) = %v, want %v", goos, got, want)
		}
	}

	// The predicate alone does not prove the command uses it: runtime.GOOS is
	// baked in at the call site, so a dropped guard would only show up on a Mac.
	installed := []string{"/opt/homebrew/bin/govard-desktop"}
	decisions := []struct {
		name    string
		goos    string
		targets []string
		want    string
	}{
		{"darwin with an installed desktop binary", "darwin", installed, "unsupported"},
		{"windows with an installed desktop binary", "windows", installed, "unsupported"},
		{"linux with an installed desktop binary", "linux", installed, "added"},
		{"linux with no installed desktop binary", "linux", nil, "no-desktop-binary"},
		{"darwin with no installed desktop binary", "darwin", nil, "no-desktop-binary"},
	}
	for _, tc := range decisions {
		t.Run(tc.name, func(t *testing.T) {
			if got := cmd.DecideDesktopUpdateForTest(tc.goos, tc.targets); got != tc.want {
				t.Fatalf("decision = %q, want %q", got, tc.want)
			}
		})
	}
}
