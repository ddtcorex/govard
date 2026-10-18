package tests

import (
	"testing"

	"govard/internal/desktop"
)

// The label-less fallback parses "<project>-<service>[-<replica>]". A
// container with a custom name carries no -<digits> replica suffix, and the
// old parser dropped it entirely (it required 3+ dash parts).
func TestDesktopPkgExtractProjectAndServiceFallback(t *testing.T) {
	cases := []struct {
		name        string
		container   string
		wantProject string
		wantService string
	}{
		{"scaled", "/shop-web-1", "shop", "web"},
		{"multi-dash project", "/my-shop-db-1", "my-shop", "db"},
		{"custom name without replica", "/shop-web", "shop", "web"},
		{"unknown service keeps last segment", "/shop-worker-1", "shop", "worker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project, service := desktop.ExtractProjectAndServiceForTest([]string{tc.container})
			if project != tc.wantProject || service != tc.wantService {
				t.Fatalf("extract(%q) = (%q, %q), want (%q, %q)",
					tc.container, project, service, tc.wantProject, tc.wantService)
			}
		})
	}
}
