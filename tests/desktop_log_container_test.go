package tests

import (
	"testing"

	"govard/internal/desktop"
)

// Without a php service the log target must still be stable: map iteration
// order made it flip between services across calls.
func TestDesktopPkgResolveLogContainerIsDeterministic(t *testing.T) {
	first := desktop.ResolveLogContainerForTest("sample", []string{"web", "db", "redis"})
	for i := 0; i < 50; i++ {
		if got := desktop.ResolveLogContainerForTest("sample", []string{"web", "db", "redis"}); got != first {
			t.Fatalf("log container flipped between calls: %q vs %q", first, got)
		}
	}
	if want := "sample-web-1"; first != want {
		t.Fatalf("expected the web service to win the fallback, got %q", first)
	}
}
