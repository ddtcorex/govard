package engine

import (
	"strings"
	"testing"
)

func storeDomainConfig(host, code string) StoreDomainMappings {
	return StoreDomainMappings{host: {Code: code}}
}

func TestValidateStoreDomainsRejectsUnsafeHost(t *testing.T) {
	for _, host := range []string{"a.test;", "a.test}", "$host.test", "a.test{"} {
		if err := validateStoreDomains(storeDomainConfig(host, "fr")); err == nil {
			t.Fatalf("host %q was accepted, want a validation error", host)
		} else if !strings.Contains(err.Error(), "may only contain letters, digits, dots and hyphens") {
			t.Fatalf("host %q error %q does not name the allowed characters", host, err)
		}
	}
}

func TestValidateStoreDomainsRejectsUnsafeCode(t *testing.T) {
	for _, code := range []string{"fr;", "1fr", "fr code", "fr-store", ""} {
		if err := validateStoreDomains(storeDomainConfig("fr.sample-project.test", code)); err == nil {
			t.Fatalf("code %q was accepted, want a validation error", code)
		} else if code != "" && !strings.Contains(err.Error(), "must start with a letter and contain only letters, digits and underscores") {
			t.Fatalf("code %q error %q does not name the allowed shape", code, err)
		}
	}
}

func TestValidateStoreDomainsAcceptsValid(t *testing.T) {
	valid := StoreDomainMappings{
		"fr.sample-project.test": {Code: "fr_store"},
		"sample-project.test":    {Code: "base", Type: "website"},
	}
	if err := validateStoreDomains(valid); err != nil {
		t.Fatalf("valid store domains were rejected: %v", err)
	}
}

// TestValidateConfigRejectsUnsafeStoreDomain pins the call site: the rule is
// worthless if ValidateConfig stops calling it.
func TestValidateConfigRejectsUnsafeStoreDomain(t *testing.T) {
	cfg := Config{
		ProjectName:  "sample-project",
		Framework:    "custom",
		Domain:       "sample-project.test",
		StoreDomains: storeDomainConfig("a.test;", "fr"),
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("ValidateConfig accepted a store domain host with a semicolon")
	}
	if !strings.Contains(err.Error(), "may only contain letters, digits, dots and hyphens") {
		t.Fatalf("error %q does not come from the store_domains rule", err)
	}
}
