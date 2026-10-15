package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopWithCommandOutputKeepsShortOutputUntouched(t *testing.T) {
	got := desktop.WithCommandOutputForTest("Environment started.", "line1\nline2")
	if got != "Environment started.\nline1\nline2" {
		t.Fatalf("short output must pass through untouched, got %q", got)
	}
}

func TestDesktopWithCommandOutputTruncatesLongOutputKeepingTail(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "log line")
	}
	lines = append(lines, "final marker")
	output := strings.Join(lines, "\n")

	got := desktop.WithCommandOutputForTest("Environment started.", output)
	if !strings.Contains(got, "final marker") {
		t.Fatalf("truncated output must keep the tail, got %q", got[len(got)-200:])
	}
	if strings.Count(got, "\n") > 41 {
		t.Fatalf("truncated output must stay capped, got %d lines", strings.Count(got, "\n")+1)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("truncated output must say so, got %q", got[:200])
	}
	if strings.Contains(got, "log line\nlog line\nlog line\nlog line\nlog line\nlog line\nlog line\nlog line\nlog line\nlog line\nlog line") && strings.Count(got, "log line") > 40 {
		t.Fatalf("truncated output must drop the head, kept %d lines", strings.Count(got, "log line"))
	}
}
