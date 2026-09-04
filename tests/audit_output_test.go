package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
)

// textLintBackend reports the configured report for every invocation, so
// command-level text rendering can be exercised without containers.
type textLintBackend struct {
	report audit.LintReport
}

func (backend *textLintBackend) Name() string { return "ci" }

func (backend *textLintBackend) Run(_ context.Context, request audit.LintRequest) (audit.LintReport, error) {
	return backend.report, nil
}

func failingLintReportWithFindings(projectID string) audit.LintReport {
	report := passingLintReport(projectID)
	report.Status = "failed"
	php := report.PHPResults[0]
	php.Outcome = "failed"
	php.Phases[0].Status = "failed"
	php.Findings = []audit.LintFinding{
		{Tool: "phpcs", Rule: "Squiz.Classes.ClassFileName", Path: "app/code/Acme/Catalog/Model/Item.php", Line: 12, Message: "Class name is not camel case"},
	}
	report.PHPResults[0] = php
	return report
}

func TestAuditRunTextSurfacesToolingLimitationBucket(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	report := failingLintReportWithFindings("audit-shop")
	php := report.PHPResults[0]
	php.LimitedFindings = 10
	report.PHPResults[0] = php
	backend := &textLintBackend{report: report}
	installAuditCommandDependencies(t, backend)

	output, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil {
		t.Fatalf("a failed lint must fail the process; output=%s", output)
	}
	rendered := string(output)
	if !strings.Contains(rendered, "10 tooling-limitation findings excluded") {
		t.Fatalf("failed-run text output hides the tooling-limitation bucket:\n%s", rendered)
	}
}

func TestAuditRunDefaultTextShowsReadableSummary(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: passingLintReport("audit-shop")}
	installAuditCommandDependencies(t, backend)

	output, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err != nil {
		t.Fatalf("default-format run returned error: %v; output=%s", err, output)
	}
	rendered := string(output)
	for _, want := range []string{
		"Audit run run-0001",
		"PASSED",
		"Scope",
		"Checks",
		"lint - passed",
		"PHP 8.4",
		"provider ci",
		"0 findings",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("default text output is missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "SchemaVersion:") || strings.Contains(rendered, "{") {
		t.Fatalf("default text output still dumps the raw struct:\n%s", rendered)
	}
	if strings.Contains(rendered, "\x1b[") {
		t.Fatalf("non-terminal writer received ANSI escape codes:\n%s", rendered)
	}
}

func TestAuditRunTextFailsProcessAndExplainsFindings(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: failingLintReportWithFindings("audit-shop")}
	installAuditCommandDependencies(t, backend)

	output, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil {
		t.Fatalf("a failed lint must fail the process; output=%s", output)
	}
	if !strings.Contains(err.Error(), "failed checks") {
		t.Fatalf("error = %v, want a failed-checks outcome error", err)
	}
	rendered := string(output)
	for _, want := range []string{
		"FAILED",
		"failed",
		"phpcs Squiz.Classes.ClassFileName app/code/Acme/Catalog/Model/Item.php:12",
		"govard audit rerun --session",
		"What next",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("failed-run text output is missing %q:\n%s", want, rendered)
		}
	}
}

func TestAuditCancelledRunTextExplainsCancellation(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: func() audit.LintReport {
		report := passingLintReport("audit-shop")
		report.Status = "cancelled"
		php := report.PHPResults[0]
		php.Outcome = "cancelled"
		php.Phases[0].Status = "cancelled"
		report.PHPResults[0] = php
		return report
	}()}
	installAuditCommandDependencies(t, backend)

	output, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error = %v, want a cancelled outcome error", err)
	}
	rendered := string(output)
	if !strings.Contains(rendered, "CANCELLED") {
		t.Fatalf("cancelled-run text output is missing CANCELLED:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Re-run:") {
		t.Fatalf("cancelled-run text output lacks a re-run hint:\n%s", rendered)
	}
}

func TestAuditStatusDefaultTextListsRunsAndInspectHint(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: passingLintReport("audit-shop")}
	installAuditCommandDependencies(t, backend)

	runOutput, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(runOutput, &result); err != nil {
		t.Fatal(err)
	}

	output, err := executeAuditCommand(t, project, []string{"audit", "status", "--session", result.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(output)
	for _, want := range []string{
		"Audit session " + result.SessionID,
		"Runs",
		"run-0001",
		"govard audit result --session " + result.SessionID,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("status text output is missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "SchemaVersion:") {
		t.Fatalf("status text output still dumps the raw manifest:\n%s", rendered)
	}
}

func TestAuditResultDefaultTextRerendersStoredEvidence(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: failingLintReportWithFindings("audit-shop")}
	installAuditCommandDependencies(t, backend)

	runOutput, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err == nil || !strings.Contains(err.Error(), "failed checks") {
		t.Fatalf("error = %v, want a failed-checks outcome error", err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(runOutput, &result); err != nil {
		t.Fatal(err)
	}

	// The stored result decodes evidence into generic maps, so rendering a
	// persisted run exercises the second branch of the evidence normalizer.
	output, _ := executeAuditCommand(t, project, []string{"audit", "result", "--session", result.SessionID, "--run", result.RunID})
	rendered := string(output)
	for _, want := range []string{
		"Audit run " + result.RunID,
		"FAILED",
		"PHP 8.4",
		"phpcs Squiz.Classes.ClassFileName app/code/Acme/Catalog/Model/Item.php:12",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("result text output is missing %q:\n%s", want, rendered)
		}
	}
}

func TestAuditCleanupDefaultTextReportsRemovedSessions(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	backend := &textLintBackend{report: passingLintReport("audit-shop")}
	installAuditCommandDependencies(t, backend)

	if _, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"}); err != nil {
		t.Fatal(err)
	}

	output, err := executeAuditCommand(t, project, []string{"audit", "cleanup", "--older-than", "1ns"})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(output)
	if !strings.Contains(rendered, "Audit cleanup") || !strings.Contains(rendered, "Removed") {
		t.Fatalf("cleanup text output is missing the removal summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "map[removed_sessions:") {
		t.Fatalf("cleanup text output still dumps the raw map:\n%s", rendered)
	}
}

func TestFindingColoredStringContainsANSIAndLocation(t *testing.T) {
	finding := audit.LintFinding{Tool: "phpcs", Rule: "Magento2.Classes.AbstractApi", Path: "app/code/Acme/Module/Model/Item.php", Line: 42, Column: 5, Message: "AbstractApi sniff violation"}
	colored := cmd.FindingColoredStringForTest(finding)
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("colored finding lacks ANSI codes: %q", colored)
	}
	for _, want := range []string{"phpcs", "Magento2.Classes.AbstractApi", "app/code/Acme/Module/Model/Item.php:42:5"} {
		if !strings.Contains(colored, want) {
			t.Fatalf("colored finding missing %q: %q", want, colored)
		}
	}
	// Non-terminal and NO_COLOR must stay plain
	if cmd.AuditColorEnabledForTest(&bytes.Buffer{}) {
		t.Fatalf("bytes.Buffer must not be considered a terminal")
	}
	t.Setenv("NO_COLOR", "1")
	if cmd.AuditColorEnabledForTest(&bytes.Buffer{}) {
		t.Fatalf("NO_COLOR must disable color even for a terminal-like writer")
	}
}
