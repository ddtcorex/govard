package tests

import (
	"fmt"
	"strings"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestFrameworkRegistryResolvesAuditTarget(t *testing.T) {
	request := types.AuditTargetResolveRequest{StartPath: t.TempDir()}

	t.Run("prefers the most specific framework in one lineage", func(t *testing.T) {
		resolver := func(types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
			return types.AuditTarget{Framework: "parent", Mode: types.AuditTargetProject}, true, nil
		}
		registry, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{
			{Parent: "parent", Definition: types.FrameworkDefinition{Name: "child"}},
			{Definition: types.FrameworkDefinition{Name: "parent", AuditTargetResolver: resolver}},
		})
		if err != nil {
			t.Fatalf("NewRegistryFromSpecs() error = %v", err)
		}

		definition, target, err := registry.ResolveAuditTarget(request)
		if err != nil {
			t.Fatalf("ResolveAuditTarget() error = %v", err)
		}
		if definition.Name != "child" {
			t.Errorf("definition.Name = %q, want child", definition.Name)
		}
		if target.Framework != "child" {
			t.Errorf("target.Framework = %q, want child", target.Framework)
		}
	})

	t.Run("rejects unrelated matching frameworks", func(t *testing.T) {
		registry := frameworks.NewRegistry()
		for _, name := range []string{"first", "second"} {
			registry.Register(types.FrameworkDefinition{
				Name: name,
				AuditTargetResolver: func(types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
					return types.AuditTarget{}, true, nil
				},
			})
		}

		_, _, err := registry.ResolveAuditTarget(request)
		if err == nil {
			t.Fatal("ResolveAuditTarget() error = nil, want unrelated ambiguity error")
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("ResolveAuditTarget() error = %q, want ambiguity context", err)
		}
	})

	t.Run("returns a resolver error from a recognized framework", func(t *testing.T) {
		registry := frameworks.NewRegistry()
		registry.Register(types.FrameworkDefinition{
			Name: "broken",
			AuditTargetResolver: func(types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
				return types.AuditTarget{}, true, fmt.Errorf("invalid target override")
			},
		})

		_, _, err := registry.ResolveAuditTarget(request)
		if err == nil || !strings.Contains(err.Error(), "invalid target override") {
			t.Errorf("ResolveAuditTarget() error = %v, want resolver error", err)
		}
	})
}

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
	// default instance (populated later by all_generated.go's init(), once it
	// exists). This test only confirms that constructing a fresh
	// NewRegistry() never touches that shared default, so tests in this
	// file can never pollute it (or be polluted by it).
	reg := frameworks.NewRegistry()
	reg.Register(types.FrameworkDefinition{Name: "isolated-test-only"})

	if _, ok := frameworks.Get("isolated-test-only"); ok {
		t.Error("registering on a fresh Registry must not affect the package-level default registry")
	}
}
