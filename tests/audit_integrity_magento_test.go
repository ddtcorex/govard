package tests

import (
	"path/filepath"
	"testing"

	"govard/internal/audit"
)

func analyzeMagentoFixture(t *testing.T, name string) []audit.LintFinding {
	t.Helper()
	analyzers, err := audit.IntegrityAnalyzers([]string{"magento-module-di"})
	if err != nil {
		t.Fatalf("IntegrityAnalyzers: %v", err)
	}
	root := filepath.Join("fixtures", "integrity", name)
	findings, err := analyzers[0].Analyze(audit.IntegrityRequest{
		ProjectRoot: root,
		TargetPath:  root,
		Framework:   "magento2",
	})
	if err != nil {
		t.Fatalf("Analyze(%s): %v", name, err)
	}
	return findings
}

func TestMagentoIntegrityValidModuleIsClean(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-valid")
	if len(findings) != 0 {
		t.Fatalf("expected a clean module, got %v", rulesOf(findings))
	}
}

func TestMagentoIntegrityModuleNameMismatch(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-name-mismatch")
	assertRule(t, findings, "MAGENTO_MODULE_NAME_MISMATCH")
}

func TestMagentoIntegrityMalformedXML(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-xml-invalid")
	assertRule(t, findings, "MAGENTO_XML_INVALID")
}

func TestMagentoIntegrityDuplicateDITarget(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-di-duplicate")
	assertRule(t, findings, "MAGENTO_DI_DUPLICATE_TARGET")
}

func TestMagentoIntegritySequenceUnknownModule(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-sequence-unknown")
	assertRule(t, findings, "MAGENTO_SEQUENCE_UNKNOWN_MODULE")
}

func TestMagentoIntegrityIgnoresNonModuleTrees(t *testing.T) {
	findings := analyzeMagentoFixture(t, "composer-valid")
	if len(findings) != 0 {
		t.Fatalf("expected no findings outside a Magento tree, got %v", rulesOf(findings))
	}
}
