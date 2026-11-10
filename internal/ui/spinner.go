package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// Spinner is the part of a pterm spinner this repo uses: the verdict at the end
// of a long operation, plus whether it animates at all. Call sites never drive
// the animation themselves.
//
// It is an interface so the pterm dependency can leave without touching the call
// sites, and so a test can tell the two implementations apart honestly instead
// of probing a concrete type.
type Spinner interface {
	Success(text ...any)
	Fail(text ...any)
	Warning(text ...any)
	Animated() bool
}

// StartSpinner starts an animated spinner on w when w is an interactive
// terminal, and returns an inert one otherwise.
//
// The guard is not cosmetic. pterm's SpinnerPrinter runs a goroutine that reads
// the same fields Stop writes without synchronization: a data race under -race
// and a torn frame in production, caught by the race detector during the unit
// suite (pterm v0.12.83, spinner_printer.go). Animating only for a human keeps
// that goroutine out of pipes, CI logs and tests, where the frames are noise.
//
// w is required and is normally the command's stderr: stdout carries the
// machine-readable output, and a decorated verdict line printed into the same
// stream makes that output unparseable (root.go states the same contract).
func StartSpinner(w io.Writer, text ...any) Spinner {
	if w == nil {
		w = io.Discard
	}
	message := strings.TrimSpace(plainText(text...))
	if !IsTerminal(w) {
		return &inertSpinner{out: w, text: message}
	}
	spinner, err := pterm.DefaultSpinner.WithWriter(w).Start(text...)
	// v0.12.83 always answers a nil error, so this is defensive: a future pterm
	// that fails to start must degrade to the inert spinner, not to a nil deref.
	if err != nil || spinner == nil {
		return &inertSpinner{out: w, text: message}
	}
	return &ptermSpinner{spinner: spinner, text: message}
}

// IsTerminal reports whether w is an interactive terminal. It exists so the
// repo has one answer: os.ModeCharDevice is not that test (/dev/null is a
// character device too, and a spinner there would start the racy goroutine for
// nobody).
func IsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok || file == nil {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// ptermSpinner adapts the pterm spinner, carrying the start text so a verdict
// called with no message still says what it is concluding.
type ptermSpinner struct {
	spinner *pterm.SpinnerPrinter
	text    string
}

func (s *ptermSpinner) Animated() bool { return true }

func (s *ptermSpinner) Success(text ...any) { s.spinner.Success(anyArgs(s.verdict(text))...) }

func (s *ptermSpinner) Fail(text ...any) { s.spinner.Fail(anyArgs(s.verdict(text))...) }

func (s *ptermSpinner) Warning(text ...any) { s.spinner.Warning(anyArgs(s.verdict(text))...) }

// verdict is the message a call passes, or the spinner's own start text when the
// call passes none, which is how pterm behaves on the animated path.
func (s *ptermSpinner) verdict(text []any) string {
	if rendered := strings.TrimSpace(pterm.Sprint(text...)); rendered != "" {
		return rendered
	}
	return s.text
}

// inertSpinner prints one plain line per verdict and starts nothing.
//
// Plain, not styled: pterm decides colour from the process environment
// (NO_COLOR, TERM) rather than from the writer, so routing a message through a
// pterm printer would put escape codes into a log file or a pipe. The animated
// path keeps its styling because there a human is watching.
type inertSpinner struct {
	out  io.Writer
	text string
}

func (s *inertSpinner) Animated() bool { return false }

func (s *inertSpinner) Success(text ...any) {
	s.print("SUCCESS", text)
}

func (s *inertSpinner) Fail(text ...any) {
	s.print("ERROR", text)
}

func (s *inertSpinner) Warning(text ...any) {
	s.print("WARNING", text)
}

func (s *inertSpinner) print(prefix string, text []any) {
	message := strings.TrimSpace(plainText(text...))
	if message == "" {
		message = s.text
	}
	if message == "" {
		_, _ = fmt.Fprintln(s.out, prefix)
		return
	}
	_, _ = fmt.Fprintln(s.out, prefix+"  "+message)
}

// plainText renders a message without interpreting anything. The animated path
// may use pterm's renderer, which consumes its own markup, but a verdict in a pipe
// is a log line: caller text goes out exactly as it was given, angle brackets
// included.
func plainText(text ...any) string {
	return fmt.Sprint(text...)
}

// anyArgs keeps the []string to []any conversion in one place.
func anyArgs(messages ...string) []any {
	args := make([]any, 0, len(messages))
	for _, message := range messages {
		if message != "" {
			args = append(args, message)
		}
	}
	return args
}
