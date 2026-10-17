package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// The sync toast must show a short human message, never the raw log tail:
// an ANSI-colored log line fed through the sync pipe must surface as a
// structured sync:progress payload with the escapes stripped.
func TestSyncProgressEmitsSanitizedMessage(t *testing.T) {
	fake := &desktop.FakePlatform{}
	desktop.ScanSyncPipeForTest(fake, strings.NewReader("\x1b[32mImporting database dump\x1b[0m\n"))

	progress := fake.EventsNamed("sync:progress")
	if len(progress) == 0 {
		t.Fatalf("expected a sync:progress event, got none (events: %+v)", fake.Events())
	}
	payload, ok := progress[0].(desktop.SyncProgressPayload)
	if !ok {
		t.Fatalf("sync:progress payload is %T, want SyncProgressPayload", progress[0])
	}
	if strings.Contains(payload.Message, "\x1b") {
		t.Fatalf("expected ANSI escapes stripped from progress message, got %q", payload.Message)
	}
	if payload.Phase == "" || payload.Message == "" {
		t.Fatalf("expected phase and message to be set, got %+v", payload)
	}
}

// Raw sync lines must stay in the progress panel: the same pipe must still
// emit sync:output alongside the structured progress event.
func TestSyncPipeKeepsRawOutputForPanel(t *testing.T) {
	fake := &desktop.FakePlatform{}
	desktop.ScanSyncPipeForTest(fake, strings.NewReader("first\nsecond\n"))

	if got := joinedPayloads(t, fake, "sync:output"); got != "first\nsecond" {
		t.Fatalf("sync:output payloads = %q, want panel lines preserved", got)
	}
}

// Each sync stage maps to its structured phase with a stable percent so the
// toast can show a short human message with a spinner instead of the tail.
func TestMapSyncProgressLinePhases(t *testing.T) {
	cases := []struct {
		line    string
		phase   string
		percent int
	}{
		{"Preparing sync plan", "prepare", 5},
		{"rsync transfer sending files", "transfer", 40},
		{"Importing database dump", "db", 60},
		{"Syncing media assets", "media", 75},
		{"Finalizing sync", "finalize", 95},
	}
	for _, tc := range cases {
		payload, ok := desktop.MapSyncProgressLineForTest(tc.line)
		if !ok {
			t.Fatalf("MapSyncProgressLine(%q) = no progress, want phase %q", tc.line, tc.phase)
		}
		if payload.Phase != tc.phase || payload.Percent != tc.percent {
			t.Fatalf("MapSyncProgressLine(%q) = %+v, want phase %q percent %d",
				tc.line, payload, tc.phase, tc.percent)
		}
		if payload.Message == "" {
			t.Fatalf("MapSyncProgressLine(%q) has empty message", tc.line)
		}
	}
}

// The mapper sanitizes through the shared UI-text helper: ANSI escapes,
// control bytes, and over-long lines must never reach the toast.
func TestMapSyncProgressLineSanitizesMessage(t *testing.T) {
	payload, ok := desktop.MapSyncProgressLineForTest("\x1b[32mImporting database\x1b[0m dump\x07" + strings.Repeat("x", 500))
	if !ok {
		t.Fatalf("expected progress for db line, got none")
	}
	for _, banned := range []string{"\x1b", "\x07"} {
		if strings.Contains(payload.Message, banned) {
			t.Fatalf("expected %q stripped from message, got %q", banned, payload.Message)
		}
	}
	if len([]rune(payload.Message)) > 140 {
		t.Fatalf("expected message capped at 140 runes, got %d (%q)", len([]rune(payload.Message)), payload.Message)
	}
}

// Specific stages must win over the generic transfer fallthrough: prepare,
// db, and media are checked first so lines like "checking config file" are not
// claimed by a generic tail word.
func TestMapSyncProgressLinePrefersSpecificStagesOverTransfer(t *testing.T) {
	cases := []struct {
		line    string
		phase   string
		percent int
	}{
		{"checking config file", "prepare", 5},
		{"loading pull cache", "prepare", 5},
		{"profile loaded", "prepare", 5},
		{"pulling files from staging", "transfer", 40},
		{"copying files 45%", "transfer", 45},
		{"45% downloaded", "transfer", 45},
		{"100% complete", "finalize", 95},
	}
	for _, tc := range cases {
		payload, ok := desktop.MapSyncProgressLineForTest(tc.line)
		if !ok {
			t.Fatalf("MapSyncProgressLine(%q) = no progress, want phase %q", tc.line, tc.phase)
		}
		if payload.Phase != tc.phase || payload.Percent != tc.percent {
			t.Fatalf("MapSyncProgressLine(%q) = %+v, want phase %q percent %d",
				tc.line, payload, tc.phase, tc.percent)
		}
	}
}

// Lines matching no stage must report a neutral working phase instead of
// being coerced into transfer, so the toast never claims a stage it cannot see.
func TestMapSyncProgressLineFallsBackToWorkingPhase(t *testing.T) {
	payload, ok := desktop.MapSyncProgressLineForTest("frobnicate the wobblewobble 12345")
	if !ok {
		t.Fatalf("expected a working-phase payload for unmatched lines, got none")
	}
	if payload.Phase != "working" {
		t.Fatalf("unmatched line phase = %q, want working", payload.Phase)
	}
	if payload.Percent != 0 || payload.Message == "" {
		t.Fatalf("unmatched line payload = %+v, want percent 0 and a short message", payload)
	}
}

// The sync:failed payload must be sanitized Go-side, not rely on the
// frontend: failure text can carry ANSI escapes from the CLI's own output.
func TestSyncFailedMessageSanitizesTerminalOutput(t *testing.T) {
	got := desktop.SyncFailedMessageForTest(errors.New("\x1b[31mboom\x1b[0m failed"))
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI escapes stripped from failure message, got %q", got)
	}
	if got != "Sync failed: boom failed" {
		t.Fatalf("unexpected failure message: %q", got)
	}
}
