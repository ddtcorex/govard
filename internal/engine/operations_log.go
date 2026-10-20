package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	OperationsLogPathEnvVar = "GOVARD_OPERATIONS_LOG_PATH"
)

type OperationStatus string

const (
	OperationStatusStart   OperationStatus = "start"
	OperationStatusSuccess OperationStatus = "success"
	OperationStatusFailure OperationStatus = "failure"
	OperationStatusPlan    OperationStatus = "plan"
)

type OperationEvent struct {
	Timestamp   string          `json:"timestamp"`
	Operation   string          `json:"operation"`
	Status      OperationStatus `json:"status"`
	Project     string          `json:"project,omitempty"`
	Category    string          `json:"category,omitempty"`
	Source      string          `json:"source,omitempty"`
	Destination string          `json:"destination,omitempty"`
	DurationMS  int64           `json:"duration_ms,omitempty"`
	Message     string          `json:"message,omitempty"`
	// OpID links the event to its exact desktop history terminal record,
	// so a failure notification's Retry button replays that record and no
	// other. Empty when the writer has no record (CLI paths, older lines):
	// notifications then offer no Retry rather than scanning for a
	// substitute. Optional and omitempty, so old log lines read back
	// unchanged.
	OpID string `json:"op_id,omitempty"`
}

func operationsLogPath() string {
	if override := strings.TrimSpace(os.Getenv(OperationsLogPathEnvVar)); override != "" {
		return filepath.Clean(override)
	}
	// GetGovardHome respects GOVARD_HOME_DIR, so tests (TestMain points it
	// at a temp dir) and dev sandboxes stay hermetic instead of reading and
	// appending to the developer's real history. Unset, it is ~/.govard.
	return filepath.Join(conventions.GetGovardHome(), "operations.log")
}

func WriteOperationEvent(event OperationEvent) (err error) {
	event.Operation = strings.TrimSpace(event.Operation)
	if event.Operation == "" {
		return fmt.Errorf("operation name is required")
	}
	if event.Status == "" {
		event.Status = OperationStatusSuccess
	}
	if strings.TrimSpace(event.Timestamp) == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Project != "" {
		event.Project = strings.TrimSpace(event.Project)
	}
	event.Source = strings.TrimSpace(event.Source)
	event.Destination = strings.TrimSpace(event.Destination)
	event.Category = strings.TrimSpace(event.Category)
	event.Message = strings.TrimSpace(event.Message)
	event.OpID = strings.TrimSpace(event.OpID)

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode operation event: %w", err)
	}

	path := operationsLogPath()
	if err := os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("create operations log dir: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, conventions.SecretFilePerm)
	if err != nil {
		return fmt.Errorf("open operations log: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write operation event: %w", err)
	}
	return nil
}

func ReadOperationEvents(limit int) ([]OperationEvent, error) {
	path := operationsLogPath()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []OperationEvent{}, nil
		}
		return nil, fmt.Errorf("open operations log: %w", err)
	}
	defer file.Close()

	if limit <= 0 {
		return readAllOperationEvents(file)
	}
	return readTailOperationEvents(file, limit)
}

func readAllOperationEvents(file *os.File) ([]OperationEvent, error) {
	events := make([]OperationEvent, 0)
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadString('\n')
		if event, ok := parseOperationLine(line); ok {
			events = append(events, event)
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		return nil, fmt.Errorf("read operations log: %w", readErr)
	}
	return events, nil
}

// operationLogTailChunk sizes the backward scan window. Lines longer than
// this (long messages) simply span windows; the stitching below rejoins
// them, so the chunk only bounds memory, never line length.
const operationLogTailChunk = 64 * 1024

// readTailOperationEvents returns the last limit successfully-parsed events
// by scanning backward from EOF, so the cost scales with the limit — not
// with the total history. A full-file parse made every sync start pay for
// the whole log (seconds on a long-lived machine). Blank and malformed
// lines are skipped exactly like the full read, so both paths agree.
func readTailOperationEvents(file *os.File, limit int) ([]OperationEvent, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat operations log: %w", err)
	}
	if info.Size() == 0 {
		return []OperationEvent{}, nil
	}

	var carry []byte    // head fragment continuing toward BOF
	var newest [][]byte // complete lines, newest first
	parsed := 0
	offset := info.Size()
	for offset > 0 && parsed < limit {
		window := int64(operationLogTailChunk)
		if window > offset {
			window = offset
		}
		offset -= window
		buf := make([]byte, window)
		if _, err := file.ReadAt(buf, offset); err != nil && err != io.EOF {
			return nil, fmt.Errorf("read operations log: %w", err)
		}
		// The bytes after this window toward EOF belong to the window's
		// last line; the head element may be a fragment continuing
		// toward BOF and is carried into the next iteration.
		parts := bytes.Split(append(buf, carry...), []byte{'\n'})
		carry = parts[0]
		for i := len(parts) - 1; i >= 1; i-- {
			line := bytes.TrimSpace(parts[i])
			if len(line) == 0 {
				continue
			}
			newest = append(newest, line)
			var probe OperationEvent
			if json.Unmarshal(line, &probe) == nil {
				parsed++
			}
			if parsed >= limit {
				break
			}
		}
	}
	if parsed < limit {
		if line := bytes.TrimSpace(carry); len(line) > 0 {
			newest = append(newest, line)
		}
	}

	events := make([]OperationEvent, 0, limit)
	for i := len(newest) - 1; i >= 0; i-- {
		if event, ok := parseOperationJSON(newest[i]); ok {
			events = append(events, event)
		}
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func parseOperationLine(line string) (OperationEvent, bool) {
	return parseOperationJSON([]byte(strings.TrimSpace(line)))
}

func parseOperationJSON(raw []byte) (OperationEvent, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return OperationEvent{}, false
	}
	var event OperationEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return OperationEvent{}, false
	}
	return event, true
}
