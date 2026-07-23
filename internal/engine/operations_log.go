package engine

import (
	"bufio"
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
}

func OperationsLogPath() string {
	if override := strings.TrimSpace(os.Getenv(OperationsLogPathEnvVar)); override != "" {
		return filepath.Clean(override)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".govard", "operations.log")
	}
	return filepath.Join(".govard", "operations.log")
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

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode operation event: %w", err)
	}

	path := OperationsLogPath()
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
	path := OperationsLogPath()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []OperationEvent{}, nil
		}
		return nil, fmt.Errorf("open operations log: %w", err)
	}
	defer file.Close()

	events := make([]OperationEvent, 0)
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var event OperationEvent
			if err := json.Unmarshal([]byte(line), &event); err == nil {
				events = append(events, event)
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		return nil, fmt.Errorf("read operations log: %w", readErr)
	}

	if limit <= 0 || len(events) <= limit {
		return events, nil
	}
	return events[len(events)-limit:], nil
}
