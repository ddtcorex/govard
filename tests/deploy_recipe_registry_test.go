package tests

import (
	"testing"

	"govard/internal/frameworks"
)

// The deploy recipe reaches the pipeline through the framework registry and
// nowhere else: internal/deploy cannot import internal/frameworks without
// closing an import cycle through the framework types, so this seam is the only
// place a framework recipe can enter.
func TestDeployRecipeIsReachableThroughTheFrameworkRegistry(t *testing.T) {
	if _, ok := frameworks.DeployRecipe("laravel"); ok {
		t.Fatal("laravel reported a deploy recipe; a framework without one must report false so the caller falls back to the neutral default")
	}
	if _, ok := frameworks.DeployRecipe("totally-unknown-framework"); ok {
		t.Fatal("an unknown framework reported a deploy recipe")
	}

	recipe, ok := frameworks.DeployRecipe("magento2")
	if !ok {
		t.Fatal("magento2 reported no deploy recipe")
	}
	if recipe.ID != "magento2" {
		t.Fatalf("magento2 recipe ID = %q, want %q", recipe.ID, "magento2")
	}
	if len(recipe.Tasks) == 0 {
		t.Fatal("magento2 recipe declares no tasks")
	}
}

// mageos inherits magento2 through types.FrameworkSpec.Parent and a patch. A
// recipe that only appears on the parent proves the threading carries through
// the inheritance path, not just through a direct definition.
func TestDeployRecipeIsInheritedByAChildFramework(t *testing.T) {
	parent, ok := frameworks.DeployRecipe("magento2")
	if !ok {
		t.Fatal("magento2 reported no deploy recipe")
	}
	child, ok := frameworks.DeployRecipe("mageos")
	if !ok {
		t.Fatal("mageos did not inherit magento2's deploy recipe")
	}
	if len(child.Tasks) != len(parent.Tasks) {
		t.Fatalf("mageos recipe has %d tasks, magento2 has %d", len(child.Tasks), len(parent.Tasks))
	}
	for _, id := range []string{"db:migrate", "build:assets", "app:cache:flush"} {
		if child.Task(id).IsEmpty() {
			t.Fatalf("mageos did not inherit the magento2 implementation of %q", id)
		}
		if child.Task(id).Command != parent.Task(id).Command {
			t.Fatalf("mageos %q command = %q, magento2 = %q", id, child.Task(id).Command, parent.Task(id).Command)
		}
	}
}
