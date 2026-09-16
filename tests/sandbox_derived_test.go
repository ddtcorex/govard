package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestDerivedProjectNameSuffixesOrigin(t *testing.T) {
	if got := deploy.DerivedProjectName("magento2-test-instance"); got != "magento2-test-instance-sandbox" {
		t.Fatalf("derived name = %q, want %q", got, "magento2-test-instance-sandbox")
	}
	if got := deploy.DerivedProjectName("My_Project"); got == "My_Project" || strings.ContainsAny(got, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("derived name must be docker-safe, got %q", got)
	}
}

func TestSandboxDomainSuffixesOrigin(t *testing.T) {
	got, err := deploy.SandboxDomain("magento2-test-instance.test")
	if err != nil {
		t.Fatalf("domain: %v", err)
	}
	if got != "magento2-test-instance-sandbox.test" {
		t.Fatalf("sandbox domain = %q, want %q", got, "magento2-test-instance-sandbox.test")
	}
	if _, err := deploy.SandboxDomain("not a domain!!"); err == nil {
		t.Fatal("a garbage origin domain must fail, not produce a garbage sandbox domain")
	}
}
