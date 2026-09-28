package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestVerifyToolItemsRegistration(t *testing.T) {
	if got := engine.VerifyToolItems("no-such-framework"); len(got) != 0 {
		t.Fatalf("VerifyToolItems for an unregistered framework = %v, want empty", got)
	}

	declared := []engine.VerifyToolItem{
		{ID: "P3-XXX-01", Phase: 3, Title: "govard tool xxx --version", Tool: "xxx", Args: []string{"--version"}},
	}
	engine.RegisterVerifyToolItems("sample-framework", declared)

	got := engine.VerifyToolItems("sample-framework")
	if len(got) != 1 || got[0].ID != "P3-XXX-01" {
		t.Fatalf("VerifyToolItems = %+v, want the declared item", got)
	}
	if got[0].Phase != 3 || got[0].Tool != "xxx" || len(got[0].Args) != 1 {
		t.Fatalf("VerifyToolItems lost fields: %+v", got[0])
	}

	// Aliases must resolve to the same block, the way the sibling
	// registrations normalize their keys.
	if alias := engine.VerifyToolItems("  Sample-Framework  "); len(alias) != 1 {
		t.Fatalf("VerifyToolItems did not normalize the key: %+v", alias)
	}
}

func TestVerifyToolItemsIgnoresEmptyRegistration(t *testing.T) {
	engine.RegisterVerifyToolItems("sample-empty-framework", []engine.VerifyToolItem{
		{ID: "P3-EMPTY-01", Phase: 3, Title: "govard tool empty --version", Tool: "empty", Args: []string{"--version"}},
	})
	engine.RegisterVerifyToolItems("sample-empty-framework", nil)

	if got := engine.VerifyToolItems("sample-empty-framework"); len(got) != 1 {
		t.Fatalf("an empty registration blanked a declared block: %+v", got)
	}
}
