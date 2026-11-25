package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// The tunnel hostname must reach the same upstream as the project's own domain.
//
// A tunnel alias routed straight to the web container skips Varnish, so Magento's page
// cache answers with its ESI placeholders unresolved and every hole-punched block, the
// storefront menu among them, silently disappears from the page.
func TestResolveTunnelAliasTargetUsesVarnishWhenTheStackHasIt(t *testing.T) {
	config := engine.Config{
		ProjectName: "demo",
		Stack: engine.Stack{
			Features: engine.Features{Varnish: true},
		},
	}

	if got, want := cmd.ResolveTunnelAliasTarget(config), "demo-varnish-1"; got != want {
		t.Fatalf("tunnel alias target = %q, want %q", got, want)
	}
}

// A stack without Varnish has nothing to skip, so the web container stays the target.
func TestResolveTunnelAliasTargetFallsBackToWeb(t *testing.T) {
	config := engine.Config{ProjectName: "demo"}

	if got, want := cmd.ResolveTunnelAliasTarget(config), "demo-web-1"; got != want {
		t.Fatalf("tunnel alias target = %q, want %q", got, want)
	}
}
