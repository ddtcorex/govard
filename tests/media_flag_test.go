package tests

import (
	"testing"

	"govard/internal/cmd"
)

func TestResolvePositionalPathArgUsesBareTrailingArg(t *testing.T) {
	got := cmd.ResolvePositionalPathArgForTest(false, "", []string{"var/log/"})
	if got != "var/log/" {
		t.Fatalf("expected positional arg to become path, got: %q", got)
	}
}

func TestResolvePositionalPathArgIgnoredWhenPathFlagChanged(t *testing.T) {
	got := cmd.ResolvePositionalPathArgForTest(true, "app/etc/config.php", []string{"ignored"})
	if got != "app/etc/config.php" {
		t.Fatalf("expected explicit --path to win, got: %q", got)
	}
}

func TestResolvePositionalPathArgIgnoredWhenNoPositionalArgs(t *testing.T) {
	got := cmd.ResolvePositionalPathArgForTest(false, "", nil)
	if got != "" {
		t.Fatalf("expected empty path when no positional args, got: %q", got)
	}
}
