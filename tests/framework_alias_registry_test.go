package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestNormalizeFrameworkAliasResolvesRegisteredAliases(t *testing.T) {
	// magento2 and wordpress register "magento"/"wp" as aliases via their
	// own Definition() - registration already happened at package init()
	// time for the whole test binary, so we assert against that real state
	// rather than registering throwaway aliases here.
	tests := []struct {
		raw      string
		expected string
	}{
		{"magento", "magento2"},
		{"MAGENTO", "magento2"},
		{"  magento  ", "magento2"},
		{"wp", "wordpress"},
		{"m2", "magento2"},
		{"m1", "magento1"},
		{"laravel", "laravel"},
		{"totally-unknown-framework", "totally-unknown-framework"},
	}
	for _, tt := range tests {
		if got := engine.NormalizeFrameworkAlias(tt.raw); got != tt.expected {
			t.Errorf("NormalizeFrameworkAlias(%q) = %q, want %q", tt.raw, got, tt.expected)
		}
	}
}
