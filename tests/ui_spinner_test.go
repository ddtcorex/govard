package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/ui"
)

// TestInertSpinnerPrintsOnePlainLinePerVerdict pins the three things the helper
// exists for, across every writer a non-interactive run can have: the verdict
// says what it concluded, it comes out as one flat line, and it carries no
// escape codes.
//
// pterm's SpinnerPrinter runs a goroutine that reads the same fields Stop writes
// without synchronization (a data race under -race, caught in the unit suite).
// Its prisoners also decide colour from the process environment rather than from
// the writer, and it reuses the spinner's own text when a verdict passes none, so
// an inert spinner that merely forwards to a pterm printer keeps both problems.
func TestInertSpinnerPrintsOnePlainLinePerVerdict(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "output.log"))
	if err != nil {
		t.Fatalf("create the log file: %v", err)
	}
	defer func() { _ = file.Close() }()

	verdicts := []struct {
		name   string
		prefix string
		run    func(ui.Spinner, ...any)
	}{
		{"success", "SUCCESS", func(s ui.Spinner, text ...any) { s.Success(text...) }},
		{"failure", "ERROR", func(s ui.Spinner, text ...any) { s.Fail(text...) }},
		{"warning", "WARNING", func(s ui.Spinner, text ...any) { s.Warning(text...) }},
	}

	for _, verdict := range verdicts {
		// A verdict with its own message, and one that relies on the start text:
		// three real call sites pass no message, and dropping it there would leave
		// the CI log with a bare prefix.
		for _, message := range []string{"with a message", ""} {
			var out bytes.Buffer
			spinner := ui.StartSpinner(&out, "Fetching remote database size...")
			if spinner.Animated() {
				t.Fatalf("%s: a buffer writer is not a terminal, so the spinner must not animate", verdict.name)
			}

			if message == "" {
				verdict.run(spinner)
			} else {
				verdict.run(spinner, message)
			}

			written := out.String()
			want := message
			if want == "" {
				want = "Fetching remote database size..."
			}
			if !strings.Contains(written, want) {
				t.Fatalf("%s with message %q: the verdict lost its text, got %q", verdict.name, message, written)
			}
			if !strings.HasPrefix(written, verdict.prefix) {
				t.Fatalf("%s: expected the %q prefix, got %q", verdict.name, verdict.prefix, written)
			}
			if strings.Contains(written, "\r") {
				t.Fatalf("%s: an inert spinner drew animation frames: %q", verdict.name, written)
			}
			if strings.Contains(written, "\x1b[") {
				t.Fatalf("%s: a log line must not carry colour escapes: %q", verdict.name, written)
			}
			if got := strings.Count(strings.TrimSpace(written), "\n"); got != 0 {
				t.Fatalf("%s: one line expected, got %d newlines in %q", verdict.name, got, written)
			}
		}
	}
}

// TestStartSpinnerWritesOnlyToTheWriterItWasGiven pins the stream contract: the
// caller hands the spinner its output (the command's stderr), and nothing is
// printed anywhere else. stdout carries the machine-readable output, so a
// decorated verdict landing there would make it unparseable.
func TestStartSpinnerWritesOnlyToTheWriterItWasGiven(t *testing.T) {
	var out bytes.Buffer
	spinner := ui.StartSpinner(&out, "Deleting project...")
	spinner.Success("Project deleted successfully.")

	if !strings.Contains(out.String(), "Project deleted successfully.") {
		t.Fatalf("the verdict must reach the writer it was given, got %q", out.String())
	}
	if ui.IsTerminal(&out) {
		t.Fatal("a buffer is not a terminal")
	}
}

// TestIsTerminalRejectsCharacterDevicesThatAreNotTerminals pins the predicate
// itself: os.ModeCharDevice is not the test, because /dev/null is a character
// device too and a spinner there would start the racy goroutine for nobody.
func TestIsTerminalRejectsCharacterDevicesThatAreNotTerminals(t *testing.T) {
	var buffer bytes.Buffer
	if ui.IsTerminal(&buffer) {
		t.Fatal("a bytes.Buffer is not a terminal")
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()
	if ui.IsTerminal(devNull) {
		t.Fatalf("%s is a character device but not a terminal", os.DevNull)
	}

	if ui.IsTerminal(nil) {
		t.Fatal("a nil writer is not a terminal")
	}
}

// TestStartSpinnerDefaultsToNothingWhenTheWriterIsNil pins that a nil writer is
// discarded rather than silently falling back to a process-wide stream.
func TestStartSpinnerDefaultsToNothingWhenTheWriterIsNil(t *testing.T) {
	spinner := ui.StartSpinner(nil, "nothing to see")
	if spinner.Animated() {
		t.Fatal("a nil writer must not animate")
	}
	spinner.Success("silently discarded")
}

// TestInertSpinnerKeepsMessageTextVerbatim pins that the plain path does not
// interpret the message. pterm renders its own markup, and an error message is
// caller text that can legitimately contain angle brackets: routing it through
// pterm's Sprint dropped the tags and printed "boom" for "<red>boom</>", which
// is a log line that no longer says what happened.
func TestInertSpinnerKeepsMessageTextVerbatim(t *testing.T) {
	var out bytes.Buffer
	spinner := ui.StartSpinner(&out, "Cleaning up")
	spinner.Fail("delete failed for <red>boom</> path")

	written := out.String()
	if !strings.Contains(written, "<red>boom</>") {
		t.Fatalf("the message lost its text through the plain path: %q", written)
	}
	if strings.Contains(written, "\x1b[") {
		t.Fatalf("the plain path must not emit escape codes: %q", written)
	}
}
