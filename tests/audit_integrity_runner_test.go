package tests

import (
	"context"
	"testing"

	"govard/internal/audit"
	"govard/internal/frameworks/types"
)

type stubIntegrityAnalyzer struct {
	id       string
	findings []audit.LintFinding
}

func (stub stubIntegrityAnalyzer) ID() string { return stub.id }

func (stub stubIntegrityAnalyzer) Analyze(audit.IntegrityRequest) ([]audit.LintFinding, error) {
	return stub.findings, nil
}

func integrityRunRequest(analyzerID string) audit.RunRequest {
	return audit.RunRequest{
		ProjectRoot:      "/work/shop",
		ProjectID:        "project-aabbccdd",
		Scope:            audit.ScopeProject,
		Checks:           []string{"integrity"},
		Environment:      audit.EnvironmentFingerprint{Framework: "magento2", GovardVersion: "test"},
		Source:           audit.SourceFingerprint{Digest: "sha256:source"},
		Target:           types.AuditTarget{Framework: "magento2", ProjectRoot: "/work/shop", TargetPath: "/work/shop", Mode: types.AuditTargetProject},
		IntegrityProfile: types.AuditIntegrityProfile{Analyzers: []string{analyzerID}},
		StackPHPVersion:  "8.3",
	}
}

func TestIntegrityJobRunsWithoutALintBackend(t *testing.T) {
	audit.RegisterIntegrityAnalyzer(stubIntegrityAnalyzer{id: "stub-clean-analyzer"})
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:     newDeterministicAuditStore(t),
		Resources: audit.Resources{CPU: 2, MemoryMB: 1024},
	})

	result, err := runner.Run(context.Background(), integrityRunRequest("stub-clean-analyzer"))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].ID != "integrity" || result.Jobs[0].Kind != "host-analysis" {
		t.Fatalf("jobs = %#v", result.Jobs)
	}
	if result.Jobs[0].Status != audit.StatusPassed {
		t.Fatalf("job status = %s, want passed", result.Jobs[0].Status)
	}
	if result.Status != audit.StatusPassed {
		t.Fatalf("run status = %s, want passed", result.Status)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != "integrity-report" {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
}

func TestIntegrityJobFailsOnFindings(t *testing.T) {
	audit.RegisterIntegrityAnalyzer(stubIntegrityAnalyzer{
		id:       "stub-finding-analyzer",
		findings: []audit.LintFinding{{Tool: "govard-integrity", Rule: "COMPOSER_LOCK_MISSING", Message: "composer.lock is missing"}},
	})
	runner := audit.NewRunner(audit.RunnerOptions{
		Store:     newDeterministicAuditStore(t),
		Resources: audit.Resources{CPU: 2, MemoryMB: 1024},
	})

	result, err := runner.Run(context.Background(), integrityRunRequest("stub-finding-analyzer"))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != audit.StatusFailed {
		t.Fatalf("run status = %s, want failed", result.Status)
	}
	if result.Jobs[0].Status != audit.StatusFailed {
		t.Fatalf("job status = %s, want failed", result.Jobs[0].Status)
	}
	findings, ok := result.Jobs[0].Evidence["findings"].([]audit.LintFinding)
	if !ok || len(findings) != 1 || findings[0].Rule != "COMPOSER_LOCK_MISSING" {
		t.Fatalf("findings evidence = %#v", result.Jobs[0].Evidence["findings"])
	}
	for _, auditError := range result.Errors {
		if auditError.Code == "infrastructure" {
			t.Fatalf("findings must not be reported as an infrastructure error: %#v", result.Errors)
		}
	}
}
