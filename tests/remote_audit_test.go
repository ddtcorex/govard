package tests

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/remote"
)

func TestWriteAuditEventAppendsJSONLines(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "remote.log")
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	if err := remote.WriteAuditEvent(remote.AuditEvent{
		Operation: "remote.test.ssh",
		Status:    remote.RemoteAuditStatusSuccess,
		Message:   "ok",
	}); err != nil {
		t.Fatalf("write first event: %v", err)
	}
	if err := remote.WriteAuditEvent(remote.AuditEvent{
		Operation: "sync.run",
		Status:    remote.RemoteAuditStatusPlan,
		Source:    "staging",
		Message:   "plan generated",
	}); err != nil {
		t.Fatalf("write second event: %v", err)
	}

	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer file.Close()

	lines := []remote.AuditEvent{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event remote.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("parse audit line: %v", err)
		}
		lines = append(lines, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan audit log: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 audit lines, got %d", len(lines))
	}
	if lines[0].Operation != "remote.test.ssh" {
		t.Fatalf("unexpected first operation: %s", lines[0].Operation)
	}
	if lines[1].Status != remote.RemoteAuditStatusPlan {
		t.Fatalf("unexpected second status: %s", lines[1].Status)
	}
	if lines[0].Timestamp == "" || lines[1].Timestamp == "" {
		t.Fatal("expected timestamps to be auto-populated")
	}
}

func TestWriteAuditEventRequiresOperation(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", filepath.Join(tempDir, "remote.log"))

	if err := remote.WriteAuditEvent(remote.AuditEvent{Status: remote.RemoteAuditStatusSuccess}); err == nil {
		t.Fatal("expected missing operation error")
	}
}

func TestReadAuditEventsLimitAndMalformedLineHandling(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "remote.log")
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	lines := []string{
		`{"timestamp":"2026-02-12T00:00:00Z","operation":"remote.add","status":"success","remote":"staging"}`,
		`this-is-not-json`,
		`{"timestamp":"2026-02-12T00:00:01Z","operation":"sync.run","status":"plan","source":"staging","destination":"local"}`,
		`{"timestamp":"2026-02-12T00:00:02Z","operation":"db.import","status":"failure","remote":"prod"}`,
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}

	events, err := remote.ReadAuditEvents(2)
	if err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Operation != "sync.run" {
		t.Fatalf("expected first limited event sync.run, got %s", events[0].Operation)
	}
	if events[1].Operation != "db.import" {
		t.Fatalf("expected second limited event db.import, got %s", events[1].Operation)
	}
}
