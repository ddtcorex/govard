package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/audit"
)

type fakeIntegrityAnalyzer struct{ id string }

func (f fakeIntegrityAnalyzer) ID() string { return f.id }

func (f fakeIntegrityAnalyzer) Analyze(audit.IntegrityRequest) ([]audit.LintFinding, error) {
	return []audit.LintFinding{{Tool: "govard-integrity", Rule: "FAKE", Message: "fake"}}, nil
}

func TestNormalizeChecksAcceptsIntegrity(t *testing.T) {
	got, err := audit.NormalizeChecks([]string{"integrity"})
	if err != nil {
		t.Fatalf("NormalizeChecks returned error: %v", err)
	}
	if want := []string{"integrity"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized checks = %#v, want %#v", got, want)
	}
}

func TestNormalizeChecksAcceptsIntegrityAlongsideContainerChecks(t *testing.T) {
	got, err := audit.NormalizeChecks([]string{"lint", "integrity"})
	if err != nil {
		t.Fatalf("NormalizeChecks returned error: %v", err)
	}
	if want := []string{"lint", "integrity"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized checks = %#v, want %#v", got, want)
	}
}

func TestNormalizeChecksStillRejectsUnknownCheck(t *testing.T) {
	if _, err := audit.NormalizeChecks([]string{"bogus"}); err == nil {
		t.Fatal("expected an error for an unknown check")
	}
}

func TestIntegrityAnalyzersResolveRegisteredIDs(t *testing.T) {
	audit.RegisterIntegrityAnalyzer(fakeIntegrityAnalyzer{id: "fake-test-analyzer"})

	analyzers, err := audit.IntegrityAnalyzers([]string{"fake-test-analyzer"})
	if err != nil {
		t.Fatalf("IntegrityAnalyzers returned error: %v", err)
	}
	if len(analyzers) != 1 || analyzers[0].ID() != "fake-test-analyzer" {
		t.Fatalf("analyzers = %#v", analyzers)
	}
}

func TestIntegrityAnalyzersRejectUnknownID(t *testing.T) {
	_, err := audit.IntegrityAnalyzers([]string{"not-registered"})
	if err == nil {
		t.Fatal("expected an error for an unregistered analyzer")
	}
	if !strings.Contains(err.Error(), "not-registered") {
		t.Fatalf("error should name the missing analyzer, got: %v", err)
	}
}
