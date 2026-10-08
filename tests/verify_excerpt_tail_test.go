package tests

import (
	"strings"
	"testing"

	"govard/internal/verify"
)

func TestExcerptKeepsTailOnFailure(t *testing.T) {
	out := strings.Repeat("progress line\n", 100) + "rsync: connection unexpectedly closed (code 12)"
	got := verify.ExcerptForTest(out, 12)
	if !strings.HasSuffix(got, "(code 12)") {
		t.Fatalf("the failing tail must survive, got %q", got)
	}
	if !strings.HasPrefix(got, "progress line") || !strings.Contains(got, "[...") {
		t.Fatalf("head and a truncation marker must stay, got %q", got)
	}
	if len(got) > 600 {
		t.Fatalf("excerpt stays bounded, got %d bytes", len(got))
	}
}

func TestExcerptKeepsHeadOnSuccess(t *testing.T) {
	out := strings.Repeat("x", 400) + "TAIL" + strings.Repeat("y", 400)
	got := verify.ExcerptForTest(out, 0)
	if len(got) != 500 || strings.Contains(got, "TAIL") && strings.Contains(got, "[...") {
		t.Fatalf("success keeps the plain 500-byte head, got len %d", len(got))
	}
	if short := verify.ExcerptForTest("  ok \n", 1); short != "ok" {
		t.Fatalf("short output is untouched, got %q", short)
	}
}
