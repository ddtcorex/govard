package tests

import (
	"reflect"
	"testing"

	"govard/internal/frameworks"
)

func TestMagentoFrameworksExposeInheritedAuditLintProfile(t *testing.T) {
	wantProject := []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}
	wantStandalone := []string{"8.1", "8.2", "8.3", "8.4", "8.5"}

	for _, name := range []string{"magento2", "mageos"} {
		t.Run(name, func(t *testing.T) {
			definition, ok := frameworks.Get(name)
			if !ok {
				t.Fatalf("framework %q is not registered", name)
			}
			if definition.AuditLint == nil {
				t.Fatal("AuditLint is nil")
			}
			if !reflect.DeepEqual(definition.AuditLint.ProjectPHPVersions, wantProject) {
				t.Fatalf("ProjectPHPVersions = %#v, want %#v", definition.AuditLint.ProjectPHPVersions, wantProject)
			}
			if !reflect.DeepEqual(definition.AuditLint.StandalonePHPVersions, wantStandalone) {
				t.Fatalf("StandalonePHPVersions = %#v, want %#v", definition.AuditLint.StandalonePHPVersions, wantStandalone)
			}
			if definition.AuditTargetResolver == nil {
				t.Fatal("AuditTargetResolver is nil")
			}
			if definition.AuditLint.CodingStandard != "Magento2" {
				t.Fatalf("CodingStandard = %q", definition.AuditLint.CodingStandard)
			}
			if definition.AuditLint.PHPStanLevel != 5 {
				t.Fatalf("PHPStanLevel = %d, want 5", definition.AuditLint.PHPStanLevel)
			}
		})
	}
}

func TestAuditLintProfileReturnedByRegistryIsCloned(t *testing.T) {
	first, _ := frameworks.Get("magento2")
	first.AuditLint.ProjectPHPVersions[0] = "9.9"
	first.AuditLint.StandalonePHPVersions[0] = "9.9"

	second, _ := frameworks.Get("magento2")
	if second.AuditLint.ProjectPHPVersions[0] != "7.4" {
		t.Fatalf("registry state was mutated: %#v", second.AuditLint.ProjectPHPVersions)
	}
	if second.AuditLint.StandalonePHPVersions[0] != "8.1" {
		t.Fatalf("registry state was mutated: %#v", second.AuditLint.StandalonePHPVersions)
	}
}
