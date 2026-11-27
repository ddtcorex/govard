package engine

import (
	"path/filepath"
	"testing"
)

// TestComposeFilePathHashIsStable is a refactor guard, not a behavioural test:
// it pins the file names the current hash produces, because every running
// project's compose file lives under those names. Changing the hash silently
// would leave every project's containers unmanaged by the next `env up`.
func TestComposeFilePathHashIsStable(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	cases := []struct {
		profile string
		want    string
	}{
		{"", "sample-project-5c81bc2420de.yml"},
		{"staging", "sample-project-staging-707db1a47233.yml"},
	}
	for _, tc := range cases {
		got := ComposeFilePathWithProfile("/srv/sample-project", "sample-project", tc.profile)
		if base := filepath.Base(got); base != tc.want {
			t.Fatalf("profile %q produced %q, want the pinned %q", tc.profile, base, tc.want)
		}
	}
}
