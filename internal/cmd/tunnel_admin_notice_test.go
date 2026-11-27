package cmd

import (
	"strings"
	"testing"
)

// The notice sends the operator to the framework's declared admin route. A store
// configured with a custom admin URL (Magento's backend front name) is not
// detected, so the wording says it is the default rather than promising it.
func TestTunnelAdminNoticeNamesTheHostAndThePathAsADefault(t *testing.T) {
	notice := tunnelAdminNotice("quiet-river.trycloudflare.com", "admin")

	if !strings.Contains(notice, "https://quiet-river.trycloudflare.com/admin") {
		t.Fatalf("the notice must carry the tunnel admin URL, got %q", notice)
	}
	if !strings.Contains(notice, "default") || !strings.Contains(notice, "custom") {
		t.Fatalf("the notice must say the path is the framework default and a custom admin URL is not detected, got %q", notice)
	}
	if !strings.Contains(notice, "local domain") {
		t.Fatalf("the notice must say the local domain serves the storefront only, got %q", notice)
	}
}
