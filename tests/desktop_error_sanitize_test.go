package tests

import (
	"errors"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
)

// Timeout and git-help errors reach toasts, so they must carry no ANSI
// escapes or control bytes and stay bounded even when the CLI dumps pages.
func TestDesktopPkgTimeoutErrorSanitizesCommandOutput(t *testing.T) {
	raw := "\x1b[31mconnection reset\x1b[0m\x00\x07" + strings.Repeat("x", 5000)
	err := desktop.NewGovardCommandTimeoutErrorForTest(15*time.Minute, raw)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	message := err.Error()
	if strings.Contains(message, "\x1b") {
		t.Fatalf("timeout error carries ANSI escapes: %q", message[:60])
	}
	if strings.ContainsAny(message, "\x00\x07") {
		t.Fatalf("timeout error carries control bytes: %q", message[:60])
	}
	if !strings.Contains(message, "connection reset") {
		t.Fatalf("sanitizing dropped the readable text: %q", message[:120])
	}
	if !strings.Contains(message, "partially started") {
		t.Fatalf("long-timeout error lost its partial-state hint: %q", message)
	}
}

func TestDesktopPkgGitConnectionHelpSanitizesCause(t *testing.T) {
	cause := errors.New("\x1b[31mfatal: could not read\x1b[0m\x00 from remote")
	message := desktop.BuildGitConnectionHelpForTest("ssh", "git@example.test:org/repo.git", cause)
	if strings.Contains(message, "\x1b") {
		t.Fatalf("git help carries ANSI escapes: %q", message)
	}
	if strings.ContainsAny(message, "\x00") {
		t.Fatalf("git help carries control bytes: %q", message)
	}
	if !strings.Contains(message, "could not read") {
		t.Fatalf("sanitizing dropped the readable text: %q", message)
	}
}
