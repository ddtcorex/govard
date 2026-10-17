package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// Final pre-merge fix round: messages must pass the shared secret scrubber,
// and unknown statuses must fail closed.

// A failure message carrying secrets must land scrubbed in the history file
// and in the stored record: KEY=value, credentialed-URL userinfo, PEM block.
func TestFinalFix_OpHistoryMessageScrubsSecrets(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	const kvSecret = "s3cret-msg-kv"
	const dsnSecret = "s3cret-msg-dsn"
	const keyBody = "MIIBOgIBAAJBAKc2s2V5qwIDAQAB"
	message := "sync failed: password=" + kvSecret +
		" via mysql://dbuser:" + dsnSecret + "@db:3306/shop" +
		" key -----BEGIN RSA PRIVATE KEY-----\n" + keyBody + "\n-----END RSA PRIVATE KEY-----"

	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusFailed, message) {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	raw, err := os.ReadFile(desktop.OperationHistoryPathForTest())
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	for _, secret := range []string{kvSecret, dsnSecret, keyBody} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("history file stores raw message secret %q; messages must pass the shared scrubber", secret)
		}
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory = %d records, want 1", len(history))
	}
	for _, secret := range []string{kvSecret, dsnSecret, keyBody} {
		if strings.Contains(history[0].Message, secret) {
			t.Errorf("stored message carries raw secret %q; messages must pass the shared scrubber", secret)
		}
	}
	if !strings.Contains(history[0].Message, "***") {
		t.Errorf("stored message = %q, want a *** scrub marker", history[0].Message)
	}
}

// The notification toast body is the event message: it must never carry raw
// secrets, even for events written by paths that skipped the write scrub.
func TestFinalFix_OperationNotificationBodyScrubsSecrets(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	const kvSecret = "s3cret-note-kv"
	notification, ok := desktop.BuildOperationNotificationForTest(engine.OperationEvent{
		Timestamp: "2026-10-17T10:00:00Z",
		Operation: "sync",
		Status:    engine.OperationStatusFailure,
		Project:   "proj-a",
		Message:   "remote rejected password=" + kvSecret + ", retry later",
	})
	if !ok {
		t.Fatal("expected failure operation notification")
	}
	if strings.Contains(notification.Body, kvSecret) {
		t.Errorf("notification body = %q, want the secret scrubbed (toast must never show it raw)", notification.Body)
	}
}

// The human-message fallback hint embeds raw CLI text: secrets in it must be
// scrubbed like every other message surface.
func TestFinalFix_HumanOpMessageFallbackHintScrubsSecrets(t *testing.T) {
	const kvSecret = "s3cret-hint-kv"
	msg := desktop.HumanOpMessageForTest("up", errors.New("weird frobnicate failure password="+kvSecret))
	if strings.Contains(msg.Hint, kvSecret) {
		t.Errorf("fallback hint = %q, want the secret scrubbed", msg.Hint)
	}
	if !strings.Contains(msg.Hint, "***") {
		t.Errorf("fallback hint = %q, want a *** scrub marker", msg.Hint)
	}
}

// The desktop operation-event writer must scrub the stored engine message,
// so the operations log never holds raw secrets (idempotent with args scrub).
func TestFinalFix_DesktopOperationEventScrubsStoredMessage(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))

	const kvSecret = "s3cret-event-kv"
	desktop.WriteDesktopOperationEventForTest(
		"desktop.remote.test", engine.OperationStatusFailure,
		"proj-a", "origin", "", "remote said password="+kvSecret, "runtime", time.Second,
	)

	events, err := engine.ReadOperationEvents(10)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if strings.Contains(events[0].Message, kvSecret) {
		t.Errorf("stored event message = %q, want the secret scrubbed", events[0].Message)
	}
}

// Unknown or empty statuses must read failed (fail closed): completing by
// default is the most misleading choice, it would mark a lost outcome done.
func TestFinalFix_UnknownOpStatusReadsFailed(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	for _, tc := range []struct {
		name   string
		status desktop.OpStatus
	}{
		{"unknown", desktop.OpStatus("bogus")},
		{"empty", desktop.OpStatus("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "op-" + tc.name
			if !desktop.RegisterOpWithArgs(id, "proj-a", "sync", []string{"sync"}, func() {}) {
				t.Fatalf("RegisterOpWithArgs(%s) = false, want true", id)
			}
			if !desktop.FinishOpWithStatus(id, tc.status, "done") {
				t.Fatalf("FinishOpWithStatus(%s) = false, want true", id)
			}
		})
	}

	history := desktop.ListHistory(0)
	if len(history) != 2 {
		t.Fatalf("ListHistory = %d records, want 2", len(history))
	}
	for _, record := range history {
		if record.Status != desktop.OpStatusFailed {
			t.Errorf("record %q status = %q, want failed (fail closed)", record.ID, record.Status)
		}
	}
}
