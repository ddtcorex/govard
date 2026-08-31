package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"govard/internal/frameworks/types"
)

// LintToolchain identifies the exact lint policy and runtime used for a run.
// It intentionally contains no project-specific inputs; Glint tracks those
// separately inside its project/toolchain cache namespace.
type LintToolchain struct {
	Provider         string   `json:"provider"`
	Image            string   `json:"image"`
	Command          []string `json:"command,omitempty"`
	PHPVersions      []string `json:"php_versions"`
	Linters          []string `json:"linters"`
	CodingStandard   string   `json:"coding_standard"`
	PHPStanLevel     int      `json:"phpstan_level"`
	PHPStanExtension string   `json:"phpstan_extension"`
}

type LintRequest struct {
	ProjectRoot string
	ProjectID   string
	SessionID   string
	RunID       string
	Provider    string
	TargetID    string
	Target      types.AuditTarget
	RunDir      string
	CacheRoot   string
	Scope       Scope
	BaseRef     string
	Jobs        int
	Profile     types.AuditLintProfile
	// SelectedPHPVersions is the explicit execution matrix, constrained by the
	// framework policy in Profile. It is not inferred from every supported PHP.
	SelectedPHPVersions []string
	// MatrixComplete states whether SelectedPHPVersions covers the entire
	// applicable framework policy rather than a narrowed execution matrix.
	MatrixComplete bool
	// BypassResultCache asks the backend to ignore any reusable analyzer state
	// for this run. The backend, not the runner, turns it into the runner
	// argument and the "bypassed" cache evidence it must observe in return.
	BypassResultCache bool
	// StreamWriter receives live Docker log output while the backend runs. When
	// nil the backend writes only to its persisted govard-lint.log file. The
	// host command passes the terminal writer in text mode so operators see
	// phase progress (validate → prepare → phpcs/phpstan) as it happens,
	// mirroring vendor/bin/phpcs behaviour; in --format json the writer stays
	// nil so machine output is not polluted.
	StreamWriter io.Writer
}

type LintPhase struct {
	Name        string `json:"name"`
	PHPVersion  string `json:"php_version,omitempty"`
	Status      string `json:"status"`
	DurationMS  int64  `json:"duration_ms"`
	CacheState  string `json:"cache_state,omitempty"`
	CacheKey    string `json:"cache_key,omitempty"`
	CacheReason string `json:"cache_reason,omitempty"`
}

// LintFinding is a provider-normalized static-analysis finding.
type LintFinding struct {
	Tool    string `json:"tool"`
	Rule    string `json:"rule,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// LintPHPResult contains complete evidence for one requested PHP version.
type LintPHPResult struct {
	PHPVersion string        `json:"php_version"`
	Outcome    string        `json:"outcome"`
	DurationMS int64         `json:"duration_ms"`
	Cache      CacheOutcome  `json:"cache"`
	Phases     []LintPhase   `json:"phases"`
	Findings   []LintFinding `json:"findings"`
}

type LintReport struct {
	SchemaVersion       int                   `json:"schema_version"`
	Provider            string                `json:"provider"`
	SessionID           string                `json:"session_id"`
	RunID               string                `json:"run_id"`
	ProjectID           string                `json:"project_id"`
	TargetID            string                `json:"target_id"`
	TargetMode          types.AuditTargetMode `json:"target_mode"`
	TargetPath          string                `json:"target_path"`
	ImageDigest         string                `json:"image_digest"`
	ToolchainDigest     string                `json:"toolchain_digest"`
	Status              string                `json:"status"`
	DurationMS          int64                 `json:"duration_ms"`
	SelectedPHPVersions []string              `json:"selected_php_versions"`
	MatrixComplete      bool                  `json:"matrix_complete"`
	PHPResults          []LintPHPResult       `json:"php_results"`
}

type LintBackend interface {
	Name() string
	Run(context.Context, LintRequest) (LintReport, error)
}

// LintToolchainDigest returns the SHA-256 digest of a canonical toolchain.
func LintToolchainDigest(toolchain LintToolchain) string {
	canonical := toolchain
	canonical.PHPVersions = append([]string(nil), toolchain.PHPVersions...)
	canonical.Linters = append([]string(nil), toolchain.Linters...)
	canonical.Command = append([]string(nil), toolchain.Command...)
	sort.Strings(canonical.PHPVersions)
	sort.Strings(canonical.Linters)
	payload, err := json.Marshal(canonical)
	if err != nil {
		panic(fmt.Sprintf("marshal lint toolchain: %v", err))
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// PHPVersions returns a defensive copy of the explicit execution matrix.
func (request LintRequest) PHPVersions() []string {
	return append([]string(nil), request.SelectedPHPVersions...)
}

// ValidateLintReport validates the fields needed before an adapter accepts a
// container-generated report as evidence for a local audit run.
func ValidateLintReport(request LintRequest, report LintReport) error {
	if report.SchemaVersion != LintReportSchemaVersion {
		return fmt.Errorf("unsupported lint report schema version %d", report.SchemaVersion)
	}
	if report.Provider == "" || report.Provider != request.Provider {
		return fmt.Errorf("lint report provider %q does not match request provider %q", report.Provider, request.Provider)
	}
	if report.SessionID == "" || report.SessionID != request.SessionID {
		return fmt.Errorf("lint report session ID %q does not match request session ID %q", report.SessionID, request.SessionID)
	}
	if report.RunID == "" || report.RunID != request.RunID {
		return fmt.Errorf("lint report run ID %q does not match request run ID %q", report.RunID, request.RunID)
	}
	if report.ProjectID == "" || report.ProjectID != request.ProjectID {
		return fmt.Errorf("lint report project ID %q does not match request project ID", report.ProjectID)
	}
	if report.TargetID == "" || report.TargetID != request.TargetID {
		return fmt.Errorf("lint report target ID %q does not match request target ID", report.TargetID)
	}
	if report.TargetMode == "" || report.TargetMode != request.Target.Mode {
		return fmt.Errorf("lint report target mode %q does not match request target mode %q", report.TargetMode, request.Target.Mode)
	}
	if report.TargetPath == "" || report.TargetPath != request.Target.TargetPath {
		return fmt.Errorf("lint report target path %q does not match request target path %q", report.TargetPath, request.Target.TargetPath)
	}
	if report.ImageDigest == "" || report.ToolchainDigest == "" {
		return fmt.Errorf("lint report is missing exact toolchain identity")
	}
	if err := validateLintMatrix(request); err != nil {
		return err
	}
	if !sameStringSlice(report.SelectedPHPVersions, request.SelectedPHPVersions) {
		return fmt.Errorf("lint report selected PHP versions %#v do not match request %#v", report.SelectedPHPVersions, request.SelectedPHPVersions)
	}
	if report.MatrixComplete != request.MatrixComplete {
		return fmt.Errorf("lint report matrix completeness %t does not match request %t", report.MatrixComplete, request.MatrixComplete)
	}
	expected := request.PHPVersions()
	seen := make(map[string]struct{}, len(report.PHPResults))
	for _, result := range report.PHPResults {
		if _, ok := seen[result.PHPVersion]; ok {
			return fmt.Errorf("lint report duplicates PHP result %q", result.PHPVersion)
		}
		seen[result.PHPVersion] = struct{}{}
		if !containsString(expected, result.PHPVersion) {
			return fmt.Errorf("lint report contains unrequested PHP result %q", result.PHPVersion)
		}
		if !lintOutcomeAllowed(result.Outcome) {
			return fmt.Errorf("lint report PHP %q has invalid outcome %q", result.PHPVersion, result.Outcome)
		}
		if len(result.Phases) == 0 {
			return fmt.Errorf("lint report PHP %q contains no phase evidence", result.PHPVersion)
		}
		for _, phase := range result.Phases {
			if phase.Name == "" || phase.Status == "" {
				return fmt.Errorf("lint report PHP %q contains incomplete phase evidence", result.PHPVersion)
			}
		}
	}
	for _, version := range expected {
		if _, ok := seen[version]; !ok {
			return fmt.Errorf("lint report is missing PHP result %q", version)
		}
	}
	if err := validateLintReportAggregate(report); err != nil {
		return err
	}
	return nil
}

func validateLintMatrix(request LintRequest) error {
	policy := request.Profile.ProjectPHPVersions
	if request.Target.Mode == types.AuditTargetStandalone {
		policy = request.Profile.StandalonePHPVersions
	}
	if len(request.SelectedPHPVersions) == 0 {
		return fmt.Errorf("lint request has no selected PHP versions")
	}
	seen := make(map[string]struct{}, len(request.SelectedPHPVersions))
	for _, version := range request.SelectedPHPVersions {
		if _, ok := seen[version]; ok {
			return fmt.Errorf("lint request duplicates selected PHP version %q", version)
		}
		seen[version] = struct{}{}
		if !containsString(policy, version) {
			return fmt.Errorf("lint request selects unsupported PHP version %q", version)
		}
	}
	matrixComplete := request.Target.Mode != types.AuditTargetStandalone || sameStringSet(request.SelectedPHPVersions, policy)
	if request.MatrixComplete != matrixComplete {
		return fmt.Errorf("lint request matrix completeness does not match selected PHP versions")
	}
	return nil
}

func validateLintReportAggregate(report LintReport) error {
	status, err := lintAggregateStatus(report.PHPResults)
	if err != nil {
		return err
	}
	if report.Status != status {
		return fmt.Errorf("lint report aggregate status %q does not match PHP results status %q", report.Status, status)
	}
	return nil
}

func lintAggregateStatus(results []LintPHPResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("lint report contains no PHP results")
	}
	var hasCancelled, hasInfraError, hasFailed, hasPassed bool
	for _, result := range results {
		if !lintOutcomeAllowed(result.Outcome) {
			return "", fmt.Errorf("lint report PHP %q has invalid outcome %q", result.PHPVersion, result.Outcome)
		}
		switch result.Outcome {
		case "cancelled":
			hasCancelled = true
		case "infra_error":
			hasInfraError = true
		case "failed":
			hasFailed = true
		case "passed":
			hasPassed = true
		}
	}
	switch {
	case hasCancelled:
		return "cancelled", nil
	case hasInfraError:
		return "infra_error", nil
	case hasFailed:
		return "failed", nil
	case hasPassed:
		return "passed", nil
	default:
		return "unsupported", nil
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	if len(seen) != len(left) {
		return false
	}
	for _, value := range right {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func lintOutcomeAllowed(outcome string) bool {
	switch outcome {
	case "passed", "failed", "unsupported", "infra_error", "cancelled":
		return true
	default:
		return false
	}
}
