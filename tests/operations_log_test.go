package tests

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestWriteOperationEventAppendsJSONLines(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "operations.log")
	t.Setenv("GOVARD_OPERATIONS_LOG_PATH", logPath)

	if err := engine.WriteOperationEvent(engine.OperationEvent{
		Operation: "init.run",
		Status:    engine.OperationStatusStart,
		Project:   "demo",
		Message:   "started",
	}); err != nil {
		t.Fatalf("write first operation event: %v", err)
	}
	if err := engine.WriteOperationEvent(engine.OperationEvent{
		Operation: "sync.run",
		Status:    engine.OperationStatusSuccess,
		Project:   "demo",
		Message:   "completed",
	}); err != nil {
		t.Fatalf("write second operation event: %v", err)
	}

	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open operations log: %v", err)
	}
	defer file.Close()

	var lines []engine.OperationEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event engine.OperationEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("parse operation line: %v", err)
		}
		lines = append(lines, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan operations log: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 operation events, got %d", len(lines))
	}
	if lines[0].Timestamp == "" || lines[1].Timestamp == "" {
		t.Fatal("expected timestamp auto-populated")
	}
	if lines[0].Operation != "init.run" {
		t.Fatalf("unexpected first operation: %s", lines[0].Operation)
	}
	if lines[1].Status != engine.OperationStatusSuccess {
		t.Fatalf("unexpected second status: %s", lines[1].Status)
	}
}

func TestWriteOperationEventRequiresOperation(t *testing.T) {
	t.Setenv("GOVARD_OPERATIONS_LOG_PATH", filepath.Join(t.TempDir(), "operations.log"))

	if err := engine.WriteOperationEvent(engine.OperationEvent{Status: engine.OperationStatusSuccess}); err == nil {
		t.Fatal("expected missing operation error")
	}
}

func TestReadOperationEventsLimitAndMalformedLineHandling(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "operations.log")
	t.Setenv("GOVARD_OPERATIONS_LOG_PATH", logPath)

	lines := []string{
		`{"timestamp":"2026-02-20T10:00:00Z","operation":"init.run","status":"success","project":"demo"}`,
		`not-json-line`,
		`{"timestamp":"2026-02-20T10:00:01Z","operation":"up.run","status":"failure","project":"demo"}`,
		`{"timestamp":"2026-02-20T10:00:02Z","operation":"sync.run","status":"plan","project":"demo"}`,
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write operations log fixture: %v", err)
	}

	events, err := engine.ReadOperationEvents(2)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Operation != "up.run" {
		t.Fatalf("expected first limited event up.run, got %s", events[0].Operation)
	}
	if events[1].Operation != "sync.run" {
		t.Fatalf("expected second limited event sync.run, got %s", events[1].Operation)
	}
}

func TestOperationsLogDefaultsToGovardHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv(engine.OperationsLogPathEnvVar, "")

	if err := engine.WriteOperationEvent(engine.OperationEvent{
		Operation: "home.dir.probe",
		Status:    engine.OperationStatusSuccess,
		Project:   "demo",
	}); err != nil {
		t.Fatalf("write operation event: %v", err)
	}

	want := filepath.Join(home, "operations.log")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected operations log at GOVARD_HOME_DIR, stat %s: %v", want, err)
	}
	events, err := engine.ReadOperationEvents(10)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	if len(events) != 1 || events[0].Operation != "home.dir.probe" {
		t.Fatalf("expected the home-dir event back, got %+v", events)
	}
}

func TestReadOperationEventsReturnsTailAcrossChunkBoundaries(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "operations.log")
	t.Setenv(engine.OperationsLogPathEnvVar, logPath)

	var lines []string
	// A valid event larger than the tail-scan chunk, forcing multi-chunk
	// stitching: it must survive as one event, not as fragments.
	lines = append(lines, `{"timestamp":"2026-02-20T09:59:59Z","operation":"big.run","status":"success","project":"demo","message":"`+strings.Repeat("x", 200000)+`"}`)
	const total = 3000
	for i := 1; i <= total; i++ {
		lines = append(lines, `{"timestamp":"2026-02-20T10:00:00Z","operation":"seq.run","status":"success","project":"demo","message":"event-`+strconv.Itoa(i)+`"}`)
		if i%500 == 0 {
			lines = append(lines, `not-json-line`, ``)
		}
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write operations log fixture: %v", err)
	}

	events, err := engine.ReadOperationEvents(50)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	if len(events) != 50 {
		t.Fatalf("expected 50 events, got %d", len(events))
	}
	for i, event := range events {
		want := "event-" + strconv.Itoa(total-50+1+i)
		if event.Message != want {
			t.Fatalf("event[%d].Message = %q, want %q", i, event.Message, want)
		}
	}

	all, err := engine.ReadOperationEvents(0)
	if err != nil {
		t.Fatalf("read all operation events: %v", err)
	}
	// 3000 sequenced + 1 big event; garbage and blanks are skipped.
	if len(all) != total+1 {
		t.Fatalf("expected %d events for limit<=0, got %d", total+1, len(all))
	}
	if all[0].Operation != "big.run" || len(all[0].Message) != 200000 {
		t.Fatal("big multi-chunk event did not survive the tail scan intact")
	}
}
