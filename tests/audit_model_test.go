package tests

import (
	"encoding/json"
	"testing"
	"time"

	"govard/internal/audit"
)

func TestAuditRunResultJSONContract(t *testing.T) {
	result := audit.RunResult{
		SchemaVersion: audit.SchemaVersion,
		SessionID:     "20260816T120000Z-a1b2c3d4",
		RunID:         "run-0001",
		ProjectID:     "project-aabbccdd",
		Scope:         audit.ScopeProject,
		Status:        audit.StatusPassed,
		StartedAt:     time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		FinishedAt:    time.Date(2026, 8, 16, 12, 1, 0, 0, time.UTC),
		Jobs:          []audit.JobResult{},
		Artifacts:     []audit.Artifact{},
		Errors:        []audit.AuditError{},
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "session_id", "run_id", "project_id", "scope", "status", "started_at", "finished_at", "jobs", "artifacts", "errors"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON field %q is missing", key)
		}
	}
	if got := int(decoded["schema_version"].(float64)); got != 1 {
		t.Fatalf("schema_version = %d, want 1", got)
	}
}
