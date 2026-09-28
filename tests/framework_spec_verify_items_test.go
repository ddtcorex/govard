package tests

import (
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func sampleVerifyItem(id string) engine.VerifyToolItem {
	return engine.VerifyToolItem{ID: id, Phase: 5, Title: "govard tool sample " + id, Tool: "sample", Args: []string{"--version"}}
}

// A child framework starts from a struct copy of its parent, so an uncloned
// slice is shared with it: a mutation through the child would rewrite the
// parent's declaration.
func TestFrameworkSpecChildDoesNotShareVerifyToolItems(t *testing.T) {
	parent := types.FrameworkDefinition{
		Name:            "sample-parent",
		VerifyToolItems: []engine.VerifyToolItem{sampleVerifyItem("P5-PAR-01")},
	}
	child := types.FrameworkSpec{
		Parent:     "sample-parent",
		Definition: types.FrameworkDefinition{Name: "sample-child"},
	}.Resolve(parent)

	if len(child.VerifyToolItems) != 1 {
		t.Fatalf("child inherited %d verify items, want 1", len(child.VerifyToolItems))
	}
	child.VerifyToolItems[0].ID = "MUTATED"

	if parent.VerifyToolItems[0].ID == "MUTATED" {
		t.Fatal("child shares the parent's VerifyToolItems backing array")
	}
}

// Without a patch entry a child cannot drop an inherited block: an empty
// registration is ignored and frameworks.Register skips empty blocks, so the
// inherited items would run for a framework they do not describe.
func TestFrameworkSpecChildCanClearVerifyToolItems(t *testing.T) {
	parent := types.FrameworkDefinition{
		Name:            "sample-parent-clear",
		VerifyToolItems: []engine.VerifyToolItem{sampleVerifyItem("P5-PAR-02")},
	}
	child := types.FrameworkSpec{
		Parent:     "sample-parent-clear",
		Definition: types.FrameworkDefinition{Name: "sample-child-clear"},
		Patch: types.FrameworkPatch{
			VerifyToolItems: types.Clear[[]engine.VerifyToolItem](),
		},
	}.Resolve(parent)

	if len(child.VerifyToolItems) != 0 {
		t.Fatalf("Clear left %d inherited verify items: %+v", len(child.VerifyToolItems), child.VerifyToolItems)
	}
}

// Pin the one inheritance that exists today as intentional rather than
// accidental: Mage-OS is a Magento 2 derivative, so the post-restore
// `setup:db:status` check applies to it unchanged.
func TestMageOSInheritsMagento2VerifyItems(t *testing.T) {
	for _, definition := range frameworks.All() {
		if definition.Name != "mageos" {
			continue
		}
		if len(definition.VerifyToolItems) == 0 {
			t.Fatal("mageos declares no verify items; expected the inherited Magento 2 block")
		}
		if got := definition.VerifyToolItems[0].ID; got != "P5-MAG-01" {
			t.Fatalf("mageos inherited %q, want P5-MAG-01 from magento2", got)
		}
		return
	}
	t.Fatal("mageos is not registered")
}
