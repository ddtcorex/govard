package tests

import (
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// Only configurations that declare no framework items: the composed counts for
// the four real frameworks are pinned in Task 4, once they exist.
func TestRegistryForReturnsStaticRegistry(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  engine.Config
	}{
		{"empty config", engine.Config{}},
		{"unknown framework", engine.Config{Framework: "no-such-framework"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verify.RegistryFor(tc.cfg)
			if len(got) != len(verify.Registry) {
				t.Fatalf("RegistryFor(%s) = %d items, want %d", tc.name, len(got), len(verify.Registry))
			}
		})
	}
}

func TestRegistryForReturnsACopy(t *testing.T) {
	got := verify.RegistryFor(engine.Config{})
	if len(got) == 0 {
		t.Fatal("RegistryFor returned no items")
	}
	got[0].Title = "mutated"

	if verify.Registry[0].Title == "mutated" {
		t.Fatal("RegistryFor must not expose the Registry slice to callers")
	}
}
