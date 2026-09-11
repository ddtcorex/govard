package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/audit"
)

func analyzeComposerFixture(t *testing.T, name string, stackPHP string) []audit.LintFinding {
	t.Helper()
	analyzers, err := audit.IntegrityAnalyzers([]string{"composer"})
	if err != nil {
		t.Fatalf("IntegrityAnalyzers: %v", err)
	}
	root := filepath.Join("fixtures", "integrity", name)
	findings, err := analyzers[0].Analyze(audit.IntegrityRequest{
		ProjectRoot:     root,
		TargetPath:      root,
		Framework:       "magento2",
		StackPHPVersion: stackPHP,
	})
	if err != nil {
		t.Fatalf("Analyze(%s): %v", name, err)
	}
	return findings
}

func rulesOf(findings []audit.LintFinding) []string {
	rules := make([]string, 0, len(findings))
	for _, finding := range findings {
		rules = append(rules, finding.Rule)
	}
	return rules
}

func assertRule(t *testing.T, findings []audit.LintFinding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			if finding.Tool != "govard-integrity" {
				t.Fatalf("finding tool = %q, want govard-integrity", finding.Tool)
			}
			if strings.TrimSpace(finding.Message) == "" {
				t.Fatalf("finding %s has no message", rule)
			}
			return
		}
	}
	t.Fatalf("rule %s not reported; got %v", rule, rulesOf(findings))
}

func assertNoRule(t *testing.T, findings []audit.LintFinding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			t.Fatalf("rule %s must not be reported for this fixture", rule)
		}
	}
}

func TestComposerIntegrityValidProjectIsClean(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-valid", "8.3")
	if len(findings) != 0 {
		t.Fatalf("expected a clean project, got %v", rulesOf(findings))
	}
}

func TestComposerIntegrityMissingLock(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-missing-lock", "8.3")
	assertRule(t, findings, "COMPOSER_LOCK_MISSING")
}

func TestComposerIntegrityRequirementNotLocked(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-not-locked", "8.3")
	assertRule(t, findings, "COMPOSER_REQUIREMENT_NOT_LOCKED")
}

func TestComposerIntegrityDuplicatePackage(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-duplicate", "8.3")
	assertRule(t, findings, "COMPOSER_DUPLICATE_PACKAGE")
}

func TestComposerIntegrityPHPConstraintMatchesStack(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-valid", "8.3")
	assertNoRule(t, findings, "COMPOSER_PHP_CONSTRAINT_MISMATCH")
}

func TestComposerIntegrityPHPConstraintMismatch(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-php-mismatch", "8.4")
	assertRule(t, findings, "COMPOSER_PHP_CONSTRAINT_MISMATCH")
}
