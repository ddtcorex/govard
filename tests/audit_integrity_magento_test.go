package tests

import (
	"path/filepath"
	"strings"
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

func TestMagentoIntegritySequenceAcceptsVendorModules(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-sequence-vendor")
	// Magento core modules live under vendor/: a <sequence> entry naming one is
	// not an unknown module, and vendor code is not itself reviewed.
	assertNoRule(t, findings, "MAGENTO_SEQUENCE_UNKNOWN_MODULE")
	if len(findings) != 0 {
		t.Fatalf("expected a clean module, got %v", rulesOf(findings))
	}
}

func TestMagentoIntegritySequenceAcceptsVendorSrcLayout(t *testing.T) {
	findings := analyzeMagentoFixture(t, "magento-sequence-vendor-src")
	assertNoRule(t, findings, "MAGENTO_SEQUENCE_UNKNOWN_MODULE")
	if len(findings) != 0 {
		t.Fatalf("expected a clean module, got %v", rulesOf(findings))
	}
}

// pathsWithRule returns the finding paths carrying one rule, so a test can
// assert *which* file an XML well-formedness failure is reported against.
func pathsWithRule(findings []audit.LintFinding, rule string) []string {
	paths := make([]string, 0, len(findings))
	for _, finding := range findings {
		if finding.Rule == rule {
			paths = append(paths, finding.Path)
		}
	}
	return paths
}

func TestMagentoIntegrityReportsMalformedWebapiAndRoutesXML(t *testing.T) {
	// `webapi.xml` and `routes.xml` are named by the walk filter, and a filter
	// that matches a file without parsing it is worse than not matching it: it
	// reads like coverage the analyzer does not have.
	findings := analyzeMagentoFixture(t, "magento-routes/webapi-invalid")
	for _, want := range []string{
		"app/code/Acme/Web/etc/webapi.xml",
		"app/code/Acme/Web/etc/routes.xml",
	} {
		found := false
		for _, path := range pathsWithRule(findings, "MAGENTO_XML_INVALID") {
			if strings.HasSuffix(path, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s was not reported as invalid XML; findings: %v", want, pathsWithRule(findings, "MAGENTO_XML_INVALID"))
		}
	}
}

func TestMagentoIntegrityChecksRoutesInATreeWithNoModuleXML(t *testing.T) {
	// The analyzer used to give up when it found neither module.xml nor di.xml,
	// which is exactly the shape of a tree that only carries route
	// definitions: the malformed file has to be reported anyway.
	findings := analyzeMagentoFixture(t, "magento-routes/webapi-only-invalid")
	paths := pathsWithRule(findings, "MAGENTO_XML_INVALID")
	if len(paths) == 0 {
		t.Fatal("a malformed webapi.xml in a tree with no module.xml was silently accepted")
	}
	if !strings.HasSuffix(paths[0], "app/code/Acme/Solo/etc/webapi.xml") {
		t.Fatalf("finding path = %q, want the webapi.xml", paths[0])
	}
}

func TestMagentoIntegrityAcceptsWellFormedRouteDefinitions(t *testing.T) {
	// The valid module fixture gains the two files in their correct form, so a
	// rule that reports every route definition would fail here.
	findings := analyzeMagentoFixture(t, "magento-valid")
	assertNoRule(t, findings, "MAGENTO_XML_INVALID")
}
