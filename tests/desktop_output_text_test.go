package tests

import (
	"io"
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestSanitizeUITextStripsANSIEscapes(t *testing.T) {
	got := desktop.SanitizeUITextForTest("\x1b[32mOK\x1b[0m done", 200)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI escapes stripped, got %q", got)
	}
	if got != "OK done" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeUITextStripsOrphanANSIFragments(t *testing.T) {
	got := desktop.SanitizeUITextForTest("[1mTitle[0m", 200)
	if strings.Contains(got, "[1m") || strings.Contains(got, "[0m") {
		t.Fatalf("expected orphan ANSI fragments stripped, got %q", got)
	}
	if got != "Title" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeUITextStripsCarriageReturnsAndControlChars(t *testing.T) {
	got := desktop.SanitizeUITextForTest("10%\r50%\x07bell\x00nul", 200)
	for _, c := range []string{"\r", "\x07", "\x00"} {
		if strings.Contains(got, c) {
			t.Fatalf("expected control char %q stripped, got %q", c, got)
		}
	}
	if got != "10%50%bellnul" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeUITextKeepsNewlineAndTab(t *testing.T) {
	got := desktop.SanitizeUITextForTest("a\tb\nc", 200)
	if got != "a\tb\nc" {
		t.Fatalf("expected tab and newline preserved, got %q", got)
	}
}

func TestSanitizeUITextDropsInvalidUTF8(t *testing.T) {
	got := desktop.SanitizeUITextForTest(string([]byte{0xff, 0xfe, 'A', 'B'}), 200)
	if got != "AB" {
		t.Fatalf("expected invalid bytes removed, got %q", got)
	}
}

func TestSanitizeUITextTruncatesWithMarker(t *testing.T) {
	got := desktop.SanitizeUITextForTest(strings.Repeat("x", 100), 20)
	if !strings.HasSuffix(got, "\u2026(truncated)") {
		t.Fatalf("expected truncation marker suffix, got %q", got)
	}
	if len([]rune(got)) != 20 {
		t.Fatalf("expected truncated length 20 runes, got %d (%q)", len([]rune(got)), got)
	}
}

func TestSanitizeUITextLeavesShortTextUntouched(t *testing.T) {
	got := desktop.SanitizeUITextForTest("short", 200)
	if got != "short" {
		t.Fatalf("expected short text untouched, got %q", got)
	}
}

// chunkReader splits reads into tiny pieces so a multi-byte ANSI escape
// sequence straddles chunk boundaries. The pipe must join chunks before
// sanitizing, otherwise the escape leaks into the UI payload.
type chunkReader struct {
	data []byte
	size int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.size
	if n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func TestDesktopScanLogPipeSanitizesSplitANSIAcrossChunks(t *testing.T) {
	fake := &desktop.FakePlatform{}
	raw := "\x1b[32mHello\x1b[0m world\n"
	desktop.ScanLogPipeForTest(fake, &chunkReader{data: []byte(raw), size: 2}, "logs:line")

	if got := joinedPayloads(t, fake, "logs:line"); got != "Hello world" {
		t.Fatalf("logs:line payloads = %q, want ANSI stripped after join", got)
	}
}
