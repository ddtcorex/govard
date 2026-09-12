package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Release states.
const (
	StatusRunning = "running"
	StatusOK      = "ok"
	StatusFailed  = "failed"
)

// ReleaseTool marks every record govard writes. Cleanup only ever deletes a
// release carrying this marker, which is what makes running next to another
// deploy tool safe.
const ReleaseTool = "govard"

// Release is the durable record of one deploy. It is the only state the engine
// keeps: resume, rollback and status all read it rather than a side file.
type Release struct {
	SchemaVersion int            `json:"schema_version"`
	Tool          string         `json:"tool"`
	Release       string         `json:"release"`
	Revision      string         `json:"revision"`
	Branch        string         `json:"branch"`
	Repository    string         `json:"repository,omitempty"`
	CreatedAt     string         `json:"created_at"`
	CreatedBy     string         `json:"created_by"`
	CI            *CIRecord      `json:"ci,omitempty"`
	Build         BuildRecord    `json:"build"`
	Tasks         []StepRecord   `json:"tasks,omitempty"`
	Publish       PublishRecord  `json:"publish"`
	Verify        VerifyRecord   `json:"verify"`
	Database      DatabaseRecord `json:"database"`
	Status        string         `json:"status"`

	// Path is where this release lives on the target. It is local knowledge,
	// never part of the stored record.
	Path string `json:"-"`
}

// CIRecord identifies the pipeline a deploy ran in, when it ran in one. The
// record points at it so a deploy from a laptop omits the block entirely: a
// struct field would always be encoded, and `"ci":{}` claims a pipeline that does
// not exist.
type CIRecord struct {
	Pipeline string `json:"pipeline,omitempty"`
	Job      string `json:"job,omitempty"`
}

// BuildRecord describes how the release was produced. The artifact fields are
// empty for a server build, and their absence in a record written before
// artifact mode existed decodes to exactly that.
type BuildRecord struct {
	Mode       string `json:"mode,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	// ArtifactRevision is the revision the artifact was built for. It is the
	// manifest's own claim, kept next to the release it was published into so
	// "which build is live" is answerable from the record alone.
	ArtifactRevision string `json:"artifact_revision,omitempty"`
	// ArtifactFiles and ArtifactBytes are what the manifest listed.
	ArtifactFiles int   `json:"artifact_files,omitempty"`
	ArtifactBytes int64 `json:"artifact_bytes,omitempty"`
}

// StepRecord is the outcome of one step.
type StepRecord struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// PublishRecord describes how the release was activated.
type PublishRecord struct {
	Strategy        string `json:"strategy,omitempty"`
	Docroot         string `json:"docroot,omitempty"`
	PreviousRelease string `json:"previous_release,omitempty"`
}

// VerifyRecord holds the post-publish check results.
type VerifyRecord struct {
	Status string        `json:"status,omitempty"`
	Checks []CheckResult `json:"checks,omitempty"`
}

// DatabaseRecord is the pre-migration dump a release was taken with. It is what
// `govard deploy rollback --with-db` restores, and its absence is a refusal
// rather than a guess.
type DatabaseRecord struct {
	Backup string `json:"backup,omitempty"`
}

// CheckResult is one verification check.
type CheckResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// NewRelease starts a record for a release being created.
func NewRelease(release, revision, branch string) *Release {
	return &Release{
		SchemaVersion: 1,
		Tool:          ReleaseTool,
		Release:       release,
		Revision:      revision,
		Branch:        branch,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		CreatedBy:     currentActor(),
		CI:            currentCI(),
		Status:        StatusRunning,
	}
}

// NewReleaseForTest exposes NewRelease to the tests/ package.
func NewReleaseForTest(release, revision, branch string) *Release {
	return NewRelease(release, revision, branch)
}

// RecordTask appends or replaces the record for one step.
func (r *Release) RecordTask(record StepRecord) {
	for idx := range r.Tasks {
		if r.Tasks[idx].ID == record.ID {
			r.Tasks[idx] = record
			return
		}
	}
	r.Tasks = append(r.Tasks, record)
}

// WriteRelease stores the record on the target.
//
// The write goes to a temporary path and is renamed into place, for the same
// reason the symlink swap is: a reader (`govard deploy status`, monitoring) must
// never observe a half-written record.
func WriteRelease(ctx context.Context, host Host, release *Release) error {
	if release == nil {
		return fmt.Errorf("write release: nil record")
	}
	payload, err := json.Marshal(release)
	if err != nil {
		return fmt.Errorf("encode release record: %w", err)
	}

	if err := writeTargetFile(ctx, host, host.ReleaseRecordPath(release.Release), payload); err != nil {
		return fmt.Errorf("write release record %s: %w", release.Release, err)
	}
	return nil
}

// ReadRelease loads one release record from the target.
func ReadRelease(ctx context.Context, host Host, release string) (*Release, error) {
	result, err := host.Runner().Run(ctx, "cat "+Shell(host.ReleaseRecordPath(release)), RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return nil, fmt.Errorf("read release record %s: %w", release, err)
	}
	var record Release
	if err := json.Unmarshal([]byte(result.Stdout), &record); err != nil {
		return nil, fmt.Errorf("parse release record %s: %w", release, err)
	}
	record.Path = host.ReleasePath(release)
	return &record, nil
}

// IncompleteRelease returns the newest release whose record is not ok, which is
// the release a `--resume` continues. It returns nil when there is nothing to
// resume, so the caller can report that rather than guess.
func IncompleteRelease(ctx context.Context, host Host) (*Release, error) {
	result, err := host.Runner().Run(ctx, "ls -1 "+Shell(host.ReleasesPath())+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	var newest *Release
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		record, err := ReadRelease(ctx, host, name)
		if err != nil || record.Tool != ReleaseTool {
			continue
		}
		if record.Status == StatusOK {
			continue
		}
		number, convErr := strconv.Atoi(name)
		if convErr != nil {
			continue
		}
		if newest == nil || number > mustAtoi(newest.Release) {
			newest = record
		}
	}
	return newest, nil
}

func mustAtoi(value string) int {
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return number
}

// ResumeTarget returns the release a `--resume` continues, or nil when there is
// nothing unfinished.
//
// It hands back the stored record rather than a fresh one seeded from a few of
// its fields. Later steps read more of the record than "which release is this":
// `CoreVerify` switches on Publish.Strategy, `deploy rollback --with-db` reads
// Database.Backup, and the record the run rewrites carries Build and CreatedAt.
// A partial copy silently hollows all of them out, so the caller continues this
// record and never rebuilds it.
func ResumeTarget(ctx context.Context, host Host) (*Release, error) {
	return IncompleteRelease(ctx, host)
}

// AppendHistory appends one line per deploy, which is what `deploy status`
// reads across environments.
func AppendHistory(ctx context.Context, host Host, release *Release) error {
	payload, err := json.Marshal(release)
	if err != nil {
		return fmt.Errorf("encode history entry: %w", err)
	}
	// Fed through standard input for the same reason as the release record: the
	// payload must not have to fit in the command's argv. The newline is part of
	// the contract — the history is one JSON document per line, and the heredoc
	// this replaced supplied the terminator for free.
	command := fmt.Sprintf(
		"mkdir -p %s && cat >> %s",
		Shell(host.DepPath()),
		Shell(host.HistoryPath()),
	)
	if _, err := host.Runner().Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Stdin: string(payload) + "\n"}); err != nil {
		return fmt.Errorf("append deploy history: %w", err)
	}
	return nil
}

// shortCommandTimeout bounds bookkeeping commands that must never hang.
const shortCommandTimeout = 2 * time.Minute

// currentActor names whoever is deploying, for the release record. CI
// identities win over the local user because that is what an operator needs
// when reading `deploy status` after an automated deploy.
// currentCI reads the pipeline identity the running CI exposes, if any. The
// variable names are the ones the two hosted forges document; a project on
// something else deploys with an empty block rather than a wrong one.
func currentCI() *CIRecord {
	pipeline := firstEnv("CI_PIPELINE_ID", "GITHUB_RUN_ID")
	job := firstEnv("CI_JOB_NAME", "GITHUB_JOB")
	if pipeline == "" && job == "" {
		return nil
	}
	return &CIRecord{Pipeline: pipeline, Job: job}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func currentActor() string {
	for _, name := range []string{"GITLAB_USER_LOGIN", "GITHUB_ACTOR", "USER", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return "local"
}
