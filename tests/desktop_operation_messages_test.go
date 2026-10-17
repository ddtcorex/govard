package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// Known failures must map to an actionable title with a retry action: a
// sync timeout is the canonical case, raw CLI text must never reach the
// toast as the headline.
func TestHumanOpMessageSyncTimeoutIsActionable(t *testing.T) {
	msg := desktop.HumanOpMessageForTest("sync", errors.New("command timed out after 5m0s: rsync partial"))

	if !strings.Contains(msg.Title, "timed out") && !strings.Contains(msg.Title, "Timed out") {
		t.Fatalf("expected an actionable timeout title, got %q", msg.Title)
	}
	if strings.Contains(msg.Title, "timed out after 5m0s") {
		t.Fatalf("expected raw CLI text kept out of the title, got %q", msg.Title)
	}
	if msg.Hint == "" {
		t.Fatalf("expected an actionable hint, got empty")
	}
	if !hasAction(msg, "retry") {
		t.Fatalf("expected a retry action, got %+v", msg.Actions)
	}
	if !hasAction(msg, "open-logs") {
		t.Fatalf("expected an open-logs action, got %+v", msg.Actions)
	}
}

// Unknown errors fall back to sanitized raw text with a copy action: ANSI
// escapes and control bytes from CLI stderr must be stripped, and the raw
// text must sit in the hint, never unsanitized.
func TestHumanOpMessageUnknownErrorFallsBackToSanitizedCopy(t *testing.T) {
	msg := desktop.HumanOpMessageForTest("up", errors.New("\x1b[31mweird frobnicate failure\x1b[0m\x07"))

	if msg.Title == "" {
		t.Fatalf("expected a fallback title, got empty")
	}
	for _, banned := range []string{"\x1b", "\x07"} {
		if strings.Contains(msg.Title, banned) || strings.Contains(msg.Hint, banned) {
			t.Fatalf("expected control bytes stripped, got title %q hint %q", msg.Title, msg.Hint)
		}
	}
	if !strings.Contains(msg.Hint, "weird frobnicate failure") {
		t.Fatalf("expected sanitized raw text in the hint, got %q", msg.Hint)
	}
	if !hasAction(msg, "copy") {
		t.Fatalf("expected a copy action for unknown errors, got %+v", msg.Actions)
	}
	if hasAction(msg, "retry") {
		t.Fatalf("unknown errors must not offer retry, got %+v", msg.Actions)
	}
}

// Every catalogued op maps its known failures: connection, auth, docker,
// and disk failures each get an actionable title, never the raw line.
func TestHumanOpMessageCatalogCoversCommonOps(t *testing.T) {
	cases := []struct {
		op      string
		err     string
		want    string
		actions []string
	}{
		{"sync", "dial tcp: connection refused", "reach", []string{"retry", "open-logs"}},
		{"onboard", "could not resolve host github.com", "reach", []string{"retry", "open-logs"}},
		{"up", "permission denied: cannot write state", "denied", []string{"open-logs"}},
		{"pull", "cannot connect to the docker daemon", "Docker", []string{"open-logs"}},
		{"sync", "no space left on device", "disk space", []string{"open-logs"}},
	}
	for _, tc := range cases {
		msg := desktop.HumanOpMessageForTest(tc.op, errors.New(tc.err))
		if !strings.Contains(msg.Title, tc.want) {
			t.Fatalf("HumanOpMessage(%q, %q) title = %q, want it to contain %q",
				tc.op, tc.err, msg.Title, tc.want)
		}
		for _, action := range tc.actions {
			if !hasAction(msg, action) {
				t.Fatalf("HumanOpMessage(%q, %q) actions = %+v, want %q",
					tc.op, tc.err, msg.Actions, action)
			}
		}
		if strings.Contains(msg.Title, tc.err) {
			t.Fatalf("HumanOpMessage(%q, %q) leaks raw text into the title %q",
				tc.op, tc.err, msg.Title)
		}
	}
}

// A cancelled op reports the cancellation honestly instead of an error:
// no alarming "failed" title, no retry prompt for a deliberate cancel.
func TestHumanOpMessageCancelledOpIsNotAnError(t *testing.T) {
	msg := desktop.HumanOpMessageForTest("sync", context.Canceled)

	if strings.Contains(strings.ToLower(msg.Title), "failed") {
		t.Fatalf("expected no failure title for a cancel, got %q", msg.Title)
	}
	if !strings.Contains(strings.ToLower(msg.Title), "cancel") {
		t.Fatalf("expected the title to name the cancel, got %q", msg.Title)
	}
	if hasAction(msg, "retry") {
		t.Fatalf("a deliberate cancel must not prompt retry, got %+v", msg.Actions)
	}
}

// A nil error carries no failure: the message stays empty so no toast is
// ever raised for a healthy op.
func TestHumanOpMessageNilErrorStaysEmpty(t *testing.T) {
	msg := desktop.HumanOpMessageForTest("sync", nil)

	if msg.Title != "" || msg.Hint != "" || len(msg.Actions) != 0 {
		t.Fatalf("expected an empty message for nil error, got %+v", msg)
	}
}

// The fifth catalogued op is env restart: its known failures map the same
// way as the other four, with the canonical "Environment restart" headline.
func TestHumanOpMessageRestartOpIsCatalogued(t *testing.T) {
	msg := desktop.HumanOpMessageForTest("restart", errors.New("command timed out after 2m0s: stop"))

	if msg.Title != "Environment restart timed out" {
		t.Fatalf("expected the catalogued restart title, got %q", msg.Title)
	}
	if msg.Hint == "" {
		t.Fatalf("expected an actionable hint, got empty")
	}
	if !hasAction(msg, "retry") || !hasAction(msg, "open-logs") {
		t.Fatalf("expected retry + open-logs actions, got %+v", msg.Actions)
	}
}

// Auth, docker, and disk failures must NOT offer retry: retrying cannot fix
// bad credentials, a down daemon, or a full disk, so the button would lie.
// These classes expose open-logs only.
func TestHumanOpMessageUnfixableFailuresOfferNoRetry(t *testing.T) {
	cases := []struct {
		op  string
		err string
	}{
		{"sync", "permission denied (publickey)"},
		{"up", "cannot connect to the docker daemon"},
		{"pull", "write error: no space left on device"},
		{"restart", "access denied for user"},
	}
	for _, tc := range cases {
		msg := desktop.HumanOpMessageForTest(tc.op, errors.New(tc.err))
		if hasAction(msg, "retry") {
			t.Fatalf("HumanOpMessage(%q, %q) must not offer retry, got %+v",
				tc.op, tc.err, msg.Actions)
		}
		if !hasAction(msg, "open-logs") {
			t.Fatalf("HumanOpMessage(%q, %q) must offer open-logs, got %+v",
				tc.op, tc.err, msg.Actions)
		}
	}
}

func hasAction(msg desktop.OpMessage, id string) bool {
	for _, action := range msg.Actions {
		if action.ID == id {
			return true
		}
	}
	return false
}
