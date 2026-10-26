package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"govard/internal/audit"
)

func phaseReport(findings []audit.LintFinding) audit.LintReport {
	return audit.LintReport{PHPResults: []audit.LintPHPResult{{
		PHPVersion: "8.3",
		Outcome:    "failed",
		Phases: []audit.LintPhase{
			{Name: "phpcs", Status: "passed"},
			{Name: "phpstan", Status: "failed"},
		},
		Findings: findings,
	}}}
}

func phaseStatus(report audit.LintReport, name string) string {
	for _, phase := range report.PHPResults[0].Phases {
		if phase.Name == name {
			return phase.Status
		}
	}
	return ""
}

func TestLintPhaseFailedWithOnlyInternalErrorsBecomesError(t *testing.T) {
	report := phaseReport([]audit.LintFinding{{Tool: "M2-LINT-PHPSTAN", Message: "Internal error: Child process error"}})
	audit.ApplyToolingLimitationPartitionForTest(&report)
	report = audit.ReclassifyEmptyFailedPhasesForTest(report)
	if got := phaseStatus(report, "phpstan"); got != "error" {
		t.Fatalf("phpstan phase = %q, want error (no findings left)", got)
	}
	if report.PHPResults[0].Outcome != "passed" || report.PHPResults[0].LimitedFindings != 1 {
		t.Fatalf("outcome %q limited %d, want passed and 1 (reclassification never changes the outcome)", report.PHPResults[0].Outcome, report.PHPResults[0].LimitedFindings)
	}
}

func TestLintPhaseFailedWithRealFindingsStaysFailed(t *testing.T) {
	report := phaseReport([]audit.LintFinding{
		{Tool: "M2-LINT-PHPSTAN", Path: "app/code/Acme/A.php", Message: "Undefined variable $x"},
		{Tool: "M2-LINT-PHPSTAN", Message: "Internal error: Child process error"},
	})
	audit.ApplyToolingLimitationPartitionForTest(&report)
	report = audit.ReclassifyEmptyFailedPhasesForTest(report)
	if got := phaseStatus(report, "phpstan"); got != "failed" {
		t.Fatalf("phpstan phase = %q, want failed while real findings remain", got)
	}
}

func TestLintPhaseErrorFieldsDecode(t *testing.T) {
	var phase audit.LintPhase
	if err := json.Unmarshal([]byte(`{"name":"phpstan","status":"error","duration_ms":5,"exit_code":255,"stderr_tail":"PHP Fatal error: x"}`), &phase); err != nil {
		t.Fatal(err)
	}
	if phase.ExitCode == nil || *phase.ExitCode != 255 || phase.StderrTail != "PHP Fatal error: x" {
		t.Fatalf("phase = %#v, want exit 255 and the stderr tail", phase)
	}
	var plain audit.LintPhase
	if err := json.Unmarshal([]byte(`{"name":"phpcs","status":"passed","duration_ms":5}`), &plain); err != nil || plain.ExitCode != nil {
		t.Fatalf("a phase without the new fields must decode with a nil exit code: %#v %v", plain, err)
	}
	encoded, _ := json.Marshal(plain)
	if strings.Contains(string(encoded), "exit_code") || strings.Contains(string(encoded), "stderr_tail") {
		t.Fatalf("absent fields must be omitted: %s", encoded)
	}
}

func TestLintInfrastructureReasonShowsExitCodeAndStderr(t *testing.T) {
	exit := 255
	report := audit.LintReport{PHPResults: []audit.LintPHPResult{{
		PHPVersion: "8.3",
		Outcome:    "infra_error",
		Phases:     []audit.LintPhase{{Name: "phpstan", Status: "error", ExitCode: &exit, StderrTail: "earlier noise\nPHP Fatal error: stub phpstan crashed\n"}},
	}}}
	reason := audit.LintInfrastructureReasonForTest(report)
	for _, needle := range []string{"phpstan: error (exit 255)", "PHP Fatal error: stub phpstan crashed"} {
		if !strings.Contains(reason, needle) {
			t.Fatalf("reason %q lacks %q", reason, needle)
		}
	}
	if strings.Contains(reason, "earlier noise") {
		t.Fatalf("reason %q must carry only the last non-empty stderr line", reason)
	}
}
