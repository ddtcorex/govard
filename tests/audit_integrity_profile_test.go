package tests

import (
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

func TestMagentoDeclaresIntegrityAnalyzers(t *testing.T) {
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("magento2 definition is not registered")
	}
	if definition.AuditIntegrity == nil {
		t.Fatal("magento2 must declare an AuditIntegrity profile")
	}
	want := map[string]bool{"composer": true, "magento-module-di": true}
	if len(definition.AuditIntegrity.Analyzers) != len(want) {
		t.Fatalf("analyzers = %v, want %v", definition.AuditIntegrity.Analyzers, want)
	}
	for _, analyzer := range definition.AuditIntegrity.Analyzers {
		if !want[analyzer] {
			t.Fatalf("unexpected analyzer %q", analyzer)
		}
	}
}

func TestIntegrityCapabilityMirrorsProfile(t *testing.T) {
	if !engine.FrameworkSupportsAuditIntegrity("magento2") {
		t.Fatal("magento2 must report the integrity capability")
	}
	if engine.FrameworkSupportsAuditIntegrity("laravel") {
		t.Fatal("laravel does not declare an integrity profile and must not report the capability")
	}
}
