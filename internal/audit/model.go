// Package audit defines the persisted contract for Govard audit sessions.
package audit

import "time"

const SchemaVersion = 1

// LintReportSchemaVersion versions provider-generated lint evidence
// independently from persisted audit sessions.
const LintReportSchemaVersion = 2

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeDiff    Scope = "diff"
)

type Status string

const (
	StatusPending       Status = "pending"
	StatusRunning       Status = "running"
	StatusPassed        Status = "passed"
	StatusFailed        Status = "failed"
	StatusCancelled     Status = "cancelled"
	StatusNotComparable Status = "not_comparable"
)

type CacheOutcome struct {
	State  string `json:"state"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type JobResult struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Status     Status         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	DurationMS int64          `json:"duration_ms"`
	Cache      CacheOutcome   `json:"cache"`
	Evidence   map[string]any `json:"evidence,omitempty"`
}

type Artifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type AuditError struct {
	JobID   string `json:"job_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type EnvironmentFingerprint struct {
	Framework        string `json:"framework"`
	FrameworkVersion string `json:"framework_version,omitempty"`
	GovardVersion    string `json:"govard_version"`
	WebServer        string `json:"web_server,omitempty"`
}

type SourceFingerprint struct {
	GitCommit string `json:"git_commit,omitempty"`
	GitDirty  bool   `json:"git_dirty"`
	Digest    string `json:"digest"`
}

type SessionManifest struct {
	SchemaVersion int                    `json:"schema_version"`
	SessionID     string                 `json:"session_id"`
	ProjectID     string                 `json:"project_id"`
	ProjectRoot   string                 `json:"project_root"`
	Scope         Scope                  `json:"scope"`
	BaseRef       string                 `json:"base_ref,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	Environment   EnvironmentFingerprint `json:"environment"`
	Source        SourceFingerprint      `json:"source"`
	Runs          []RunReference         `json:"runs"`
}

type RunReference struct {
	RunID     string    `json:"run_id"`
	CreatedAt time.Time `json:"created_at"`
}

type RunResult struct {
	SchemaVersion int                    `json:"schema_version"`
	SessionID     string                 `json:"session_id"`
	RunID         string                 `json:"run_id"`
	ProjectID     string                 `json:"project_id"`
	Scope         Scope                  `json:"scope"`
	Status        Status                 `json:"status"`
	StartedAt     time.Time              `json:"started_at"`
	FinishedAt    time.Time              `json:"finished_at"`
	Environment   EnvironmentFingerprint `json:"environment"`
	Source        SourceFingerprint      `json:"source"`
	Jobs          []JobResult            `json:"jobs"`
	Artifacts     []Artifact             `json:"artifacts"`
	Errors        []AuditError           `json:"errors"`
}
