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

func TestSandboxSpecForOriginInheritsBlueprint(t *testing.T) {
	spec, err := deploy.SandboxSpecForOrigin(deploy.OriginProject{
		Name:      "magento2-test-instance",
		Framework: "magento2",
		PHP:       "8.5",
		Profile:   deploy.SandboxProfileFull,
		Requirements: deploy.SandboxRequirements{
			Extensions: []string{"xsl", "zip"},
			Services:   []string{"mariadb", "redis-server"},
		},
	})
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if spec.Project != "magento2-test-instance-sandbox" {
		t.Errorf("spec project = %q, want the derived name", spec.Project)
	}
	if spec.PHP != "8.5" {
		t.Errorf("spec php = %q, want the origin series (no hand flag)", spec.PHP)
	}
	for _, want := range []string{"xsl", "zip", "mariadb", "redis-server"} {
		found := false
		for _, have := range append(spec.Requirements.Extensions, spec.Requirements.Services...) {
			if have == want {
				found = true
			}
		}
		if !found {
			t.Errorf("spec lost the origin requirement %q: %+v", want, spec.Requirements)
		}
	}
}

func TestSandboxSpecForOriginRejectsEmptyPHP(t *testing.T) {
	if _, err := deploy.SandboxSpecForOrigin(deploy.OriginProject{Name: "shop"}); err == nil {
		t.Fatal("an origin with no PHP series must fail loudly, not guess one")
	}
}
