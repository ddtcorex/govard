package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgSanitizeStreamLineForTestStripsANSIAndControlChars(t *testing.T) {
	raw := []byte("\x1b[32mBootstrap\x1b[0m step \x07ok")

	got := desktop.SanitizeStreamLineForTest(raw)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI escape codes stripped, got %q", got)
	}
	if strings.Contains(got, "\x07") {
		t.Fatalf("expected control characters stripped, got %q", got)
	}
	if got != "Bootstrap step ok" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestDesktopPkgSanitizeStreamLineForTestStripsOrphanANSIFragments(t *testing.T) {
	raw := []byte("[1mSynchronization Plan Review[0m")

	got := desktop.SanitizeStreamLineForTest(raw)
	if strings.Contains(got, "[1m") || strings.Contains(got, "[0m") {
		t.Fatalf("expected orphan ANSI style fragments stripped, got %q", got)
	}
	if got != "Synchronization Plan Review" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestDesktopPkgSanitizeStreamLineForTestDropsInvalidUTF8(t *testing.T) {
	raw := []byte{0xff, 0xfe, 'A', 'B'}

	got := desktop.SanitizeStreamLineForTest(raw)
	if got != "AB" {
		t.Fatalf("expected invalid bytes removed, got %q", got)
	}
}

func joinedPayloads(t *testing.T, fake *desktop.FakePlatform, event string) string {
	t.Helper()
	var parts []string
	for _, p := range fake.EventsNamed(event) {
		s, ok := p.(string)
		if !ok {
			t.Fatalf("payload for %s is %T, want string", event, p)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

func TestDesktopScanLogPipeEmitsThroughPlatform(t *testing.T) {
	fake := &desktop.FakePlatform{}
	desktop.ScanLogPipeForTest(fake, strings.NewReader("one\ntwo\n"), "sync:output")

	if got := joinedPayloads(t, fake, "sync:output"); got != "one\ntwo" {
		t.Fatalf("sync:output payloads = %q", got)
	}
}

func TestDesktopScanLogPipeFlushesTrailingPartialLine(t *testing.T) {
	fake := &desktop.FakePlatform{}
	desktop.ScanLogPipeForTest(fake, strings.NewReader("first\nlast-without-newline"), "logs:line")

	if got := joinedPayloads(t, fake, "logs:line"); got != "first\nlast-without-newline" {
		t.Fatalf("logs:line payloads = %q", got)
	}
}

func TestDesktopScanLogPipeSplitsCarriageReturns(t *testing.T) {
	fake := &desktop.FakePlatform{}
	desktop.ScanLogPipeForTest(fake, strings.NewReader("10%\r50%\r100%\r\ndone\n"), "sync:output")

	if got := joinedPayloads(t, fake, "sync:output"); got != "10%\n50%\n100%\ndone" {
		t.Fatalf("sync:output payloads = %q", got)
	}
}
