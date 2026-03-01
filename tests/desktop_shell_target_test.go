package tests

import (
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgResolveShellServiceNameForTest(t *testing.T) {
	t.Run("all prefers web service", func(t *testing.T) {
		got := desktop.ResolveShellServiceNameForTest("all", []string{"db", "web", "redis"})
		if got != "web" {
			t.Fatalf("expected web, got %q", got)
		}
	})

	t.Run("empty request prefers php when present", func(t *testing.T) {
		got := desktop.ResolveShellServiceNameForTest("", []string{"php", "db"})
		if got != "php" {
			t.Fatalf("expected php, got %q", got)
		}
	})

	t.Run("unknown request falls back to preferred service", func(t *testing.T) {
		got := desktop.ResolveShellServiceNameForTest("unknown", []string{"web", "db"})
		if got != "web" {
			t.Fatalf("expected web, got %q", got)
		}
	})

	t.Run("unknown request falls back deterministically without preferred services", func(t *testing.T) {
		got := desktop.ResolveShellServiceNameForTest("unknown", []string{"redis", "db"})
		if got != "db" {
			t.Fatalf("expected db, got %q", got)
		}
	})
}
