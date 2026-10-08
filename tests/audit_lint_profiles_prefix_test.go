package tests

import (
	"testing"

	"govard/internal/frameworks"
)

func TestFrameworkLintProfilesDeclareRuleIDPrefix(t *testing.T) {
	want := map[string]string{
		"magento2":  "M2-LINT",
		"laravel":   "LARAVEL-LINT",
		"symfony":   "SYMFONY-LINT",
		"wordpress": "WP-LINT",
	}
	for name, prefix := range want {
		def, ok := frameworks.Get(name)
		if !ok || def.AuditLint == nil {
			t.Fatalf("%s has no lint profile", name)
		}
		if def.AuditLint.RuleIDPrefix != prefix {
			t.Errorf("%s RuleIDPrefix = %q, want %q", name, def.AuditLint.RuleIDPrefix, prefix)
		}
	}
}
