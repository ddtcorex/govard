package remote

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
	RemoteAuditStatusSuccess = "success"
	RemoteAuditStatusFailure = "failure"
	RemoteAuditStatusWarning = "warning"
	RemoteAuditStatusPlan    = "plan"
)

type AuditEvent struct {
	Timestamp   string `json:"timestamp"`
	Operation   string `json:"operation"`
	Status      string `json:"status"`
	Category    string `json:"category,omitempty"`
	Remote      string `json:"remote,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
	Message     string `json:"message,omitempty"`
}

func AuditLogPath() string {
	if override := os.Getenv("GOVARD_REMOTE_AUDIT_LOG_PATH"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".govard", "remote.log")
}

func WriteAuditEvent(event AuditEvent) (err error) {
	if event.Operation == "" {
		return fmt.Errorf("remote audit operation is required")
	}
	if event.Status == "" {
		event.Status = RemoteAuditStatusSuccess
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode remote audit event: %w", err)
	}

	path := AuditLogPath()
	if err := os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("create remote audit dir: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, conventions.SecretFilePerm)
	if err != nil {
		return fmt.Errorf("open remote audit log: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write remote audit event: %w", err)
	}
	return nil
}

func ReadAuditEvents(limit int) ([]AuditEvent, error) {
	path := AuditLogPath()
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events := make([]AuditEvent, 0)
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var event AuditEvent
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
		return nil, fmt.Errorf("read remote audit log: %w", readErr)
	}

	if limit <= 0 || len(events) <= limit {
		return events, nil
	}
	return events[len(events)-limit:], nil
}
