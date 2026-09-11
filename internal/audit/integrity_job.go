package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"govard/internal/frameworks/types"
)

// IntegrityReport is the persisted evidence of one container-free analysis run.
// Findings reuse LintFinding so downstream consumers need no second parser.
type IntegrityReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Provider      string                `json:"provider"`
	Analyzers     []string              `json:"analyzers"`
	TargetMode    types.AuditTargetMode `json:"target_mode"`
	TargetPath    string                `json:"target_path"`
	Status        string                `json:"status"`
	Findings      []LintFinding         `json:"findings"`
}

// integrityReportSchemaVersion versions the persisted report.
const integrityReportSchemaVersion = 1

// integrityReportFilename is the artifact name inside the run directory.
const integrityReportFilename = "integrity.json"

// savedIntegritySettings are persisted with the job so a rerun can rebuild the
// same analysis without the caller repeating the framework profile.
type savedIntegritySettings struct {
	Profile         types.AuditIntegrityProfile `json:"profile"`
	StackPHPVersion string                      `json:"stack_php_version,omitempty"`
	Target          types.AuditTarget           `json:"target"`
}

type integrityFailureError struct{}

func (integrityFailureError) Error() string { return "integrity analysis reported findings" }

// integrityJob builds the container-free analysis job. It never touches a
// container runtime or a PHP toolchain: analyzers read the checked-out source
// and the resolved project config only.
func (runner *Runner) integrityJob(request RunRequest, manifest SessionManifest, runID string) (Job, error) {
	analyzers, err := IntegrityAnalyzers(request.IntegrityProfile.Analyzers)
	if err != nil {
		return Job{}, err
	}
	if len(analyzers) == 0 {
		return Job{}, fmt.Errorf("integrity check requires at least one analyzer in the framework profile")
	}

	targetPath := strings.TrimSpace(request.Target.TargetPath)
	if targetPath == "" {
		targetPath = request.ProjectRoot
	}
	framework := request.Environment.Framework
	stackPHP := request.StackPHPVersion
	projectRoot := request.ProjectRoot

	run := func(ctx context.Context) (map[string]any, error) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		analyzerIDs := make([]string, 0, len(analyzers))
		findings := []LintFinding{}
		for _, analyzer := range analyzers {
			analyzerIDs = append(analyzerIDs, analyzer.ID())
			analyzerFindings, analyzeErr := analyzer.Analyze(IntegrityRequest{
				ProjectRoot:     projectRoot,
				TargetPath:      targetPath,
				Framework:       framework,
				StackPHPVersion: stackPHP,
			})
			if analyzeErr != nil {
				return nil, fmt.Errorf("integrity analyzer %s: %w", analyzer.ID(), analyzeErr)
			}
			findings = append(findings, analyzerFindings...)
		}

		status := "passed"
		if len(findings) > 0 {
			status = "failed"
		}
		report := IntegrityReport{
			SchemaVersion: integrityReportSchemaVersion,
			Provider:      integrityToolName,
			Analyzers:     analyzerIDs,
			TargetMode:    request.Target.Mode,
			TargetPath:    targetPath,
			Status:        status,
			Findings:      findings,
		}

		artifactPath, pathErr := runner.store.RunArtifactPath(manifest.ProjectID, manifest.SessionID, runID, integrityReportFilename)
		if pathErr != nil {
			return nil, fmt.Errorf("resolve integrity artifact path: %w", pathErr)
		}
		payload, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal integrity report: %w", marshalErr)
		}
		payload = append(payload, '\n')
		if writeErr := os.WriteFile(artifactPath, payload, 0o600); writeErr != nil {
			return nil, fmt.Errorf("write integrity report: %w", writeErr)
		}
		sum := sha256.Sum256(payload)

		evidence := map[string]any{
			"analyzers": analyzerIDs,
			"integrity_settings": savedIntegritySettings{
				Profile:         request.IntegrityProfile,
				StackPHPVersion: request.StackPHPVersion,
				Target:          request.Target,
			},
			"findings": findings,
			"status":   status,
			"artifact": Artifact{
				Kind:   "integrity-report",
				Path:   artifactPath,
				SHA256: hex.EncodeToString(sum[:]),
			},
		}
		if len(findings) > 0 {
			return evidence, integrityFailureError{}
		}
		return evidence, nil
	}

	return Job{
		ID:        IntegrityCheck,
		Kind:      "host-analysis",
		Resources: Resources{CPU: 1, MemoryMB: 256},
		Run:       run,
	}, nil
}

func integrityArtifacts(jobs []JobResult) []Artifact {
	for _, job := range jobs {
		if job.ID != IntegrityCheck {
			continue
		}
		if artifact, ok := job.Evidence["artifact"].(Artifact); ok {
			return []Artifact{artifact}
		}
	}
	return nil
}
