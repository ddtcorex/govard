package tests

import (
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := frameworks.NewRegistry()
	reg.Register(types.FrameworkDefinition{
		Name:        "widgetframework",
		Aliases:     []string{"widget"},
		DisplayName: "Widget Framework",
	})

	def, ok := reg.Get("widgetframework")
	if !ok {
		t.Fatal("expected widgetframework to be registered")
	}
	if def.DisplayName != "Widget Framework" {
		t.Errorf("DisplayName = %q, want %q", def.DisplayName, "Widget Framework")
	}

	aliasDef, ok := reg.Get("widget")
	if !ok {
		t.Fatal("expected alias 'widget' to resolve")
	}
	if aliasDef.Name != "widgetframework" {
		t.Errorf("alias resolved to Name %q, want %q", aliasDef.Name, "widgetframework")
	}

	if _, ok := reg.Get("nonexistent"); ok {
		t.Error("expected nonexistent framework to not be found")
	}
}

func TestRegistryAll(t *testing.T) {
	reg := frameworks.NewRegistry()
	reg.Register(types.FrameworkDefinition{Name: "one"})
	reg.Register(types.FrameworkDefinition{Name: "two"})

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d definitions, want 2", len(all))
	}

	names := map[string]bool{}
	for _, def := range all {
		names[def.Name] = true
	}
	if !names["one"] || !names["two"] {
		t.Errorf("All() = %v, want both 'one' and 'two'", all)
	}
}

func TestRegistryNormalize(t *testing.T) {
	reg := frameworks.NewRegistry()
	reg.Register(types.FrameworkDefinition{Name: "magento2", Aliases: []string{"magento"}})

	cases := []struct{ raw, want string }{
		{"magento2", "magento2"},
		{"Magento", "magento2"},
		{"  magento  ", "magento2"},
		{"unknown-framework", "unknown-framework"},
	}
	for _, tc := range cases {
		if got := reg.Normalize(tc.raw); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestRegistryPackageLevelDefaultIsIsolatedFromTestRegistries(t *testing.T) {
	// The package-level Register/Get/All/Normalize operate on a shared
	// default instance (populated later by all.go's init(), once it
	// exists). This test only confirms that constructing a fresh
	// NewRegistry() never touches that shared default, so tests in this
	// file can never pollute it (or be polluted by it).
	reg := frameworks.NewRegistry()
	reg.Register(types.FrameworkDefinition{Name: "isolated-test-only"})

	if _, ok := frameworks.Get("isolated-test-only"); ok {
		t.Error("registering on a fresh Registry must not affect the package-level default registry")
	}
}
