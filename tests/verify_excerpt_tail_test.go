package tests

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestExcerptStripsANSIEscapeSequences(t *testing.T) {
	out := "\x1b[1;31mFAIL\x1b[0m check \x1b]8;;http://x\x07link\x1b]8;;\x1b\\ done\x1b[2K\r"
	got := verify.ExcerptForTest(out, 1)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\x07") {
		t.Fatalf("excerpt still holds escape bytes: %q", got)
	}
	if got != "FAIL check link done" {
		t.Errorf("excerpt = %q, want the visible text only", got)
	}
}

func TestExcerptNeverCutsAnEscapeOrACharacterInHalf(t *testing.T) {
	// Colored head and tail: the cut points land inside escape sequences and
	// inside multi-byte runes unless the output is cleaned first.
	colored := strings.Repeat("\x1b[38;5;196mé\x1b[0m", 400)
	for _, code := range []int{0, 1} {
		got := verify.ExcerptForTest(colored, code)
		if strings.Contains(got, "\x1b") {
			t.Errorf("exit %d: escape bytes survived: %q", code, got)
		}
		if !utf8.ValidString(got) {
			t.Errorf("exit %d: excerpt is not valid UTF-8: %q", code, got)
		}
	}
	// Plain multi-byte text cut mid-rune.
	wide := strings.Repeat("é", 600)
	for _, code := range []int{0, 1} {
		if got := verify.ExcerptForTest(wide, code); !utf8.ValidString(got) {
			t.Errorf("exit %d: invalid UTF-8 after the cut: %q", code, got)
		}
	}
}
