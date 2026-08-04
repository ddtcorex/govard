package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestRegistryBuildResolvesChildBeforeParent(t *testing.T) {
	root := types.FrameworkSpec{
		Definition: types.FrameworkDefinition{
			Name:            "parent",
			Aliases:         []string{"base"},
			DisplayName:     "Parent",
			PHPImageVariant: "parent",
			PHPStanPaths:    []string{"app", "src"},
		},
	}
	child := types.FrameworkSpec{
		Parent: "parent",
		Definition: types.FrameworkDefinition{
			Name: "child",
		},
		Patch: types.FrameworkPatch{
			DisplayName:     types.Set("Child"),
			PHPImageVariant: types.Clear[string](),
			PHPStanPaths:    types.Set([]string{"packages/child"}),
		},
	}

	registry, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{child, root})
	if err != nil {
		t.Fatalf("NewRegistryFromSpecs() error = %v", err)
	}

	definition, ok := registry.Get("child")
	if !ok {
		t.Fatal("child definition was not resolved")
	}
	if definition.DisplayName != "Child" {
		t.Errorf("DisplayName = %q, want Child", definition.DisplayName)
	}
	if definition.PHPImageVariant != "" {
		t.Errorf("PHPImageVariant = %q, want explicitly cleared value", definition.PHPImageVariant)
	}
	if !reflect.DeepEqual(definition.PHPStanPaths, []string{"packages/child"}) {
		t.Errorf("PHPStanPaths = %v, want child override", definition.PHPStanPaths)
	}
	if got, want := registry.Lineage("child"), []string{"parent", "child"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lineage(child) = %v, want %v", got, want)
	}
	if !registry.IsA("child", "parent") {
		t.Error("child should be recognized as a parent descendant")
	}
}

func TestRegistryBuildRejectsInheritanceCycle(t *testing.T) {
	_, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{
		{Parent: "two", Definition: types.FrameworkDefinition{Name: "one"}},
		{Parent: "one", Definition: types.FrameworkDefinition{Name: "two"}},
	})
	if err == nil {
		t.Fatal("NewRegistryFromSpecs() succeeded for an inheritance cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle error = %q, want text containing cycle", err)
	}
}

func TestRegistryBuildAppliesRuntimeAndFeatureOverrides(t *testing.T) {
	root := types.FrameworkSpec{
		Definition: types.FrameworkDefinition{
			Name:              "parent",
			Config:            engine.FrameworkConfig{Runtime: "php", DefaultPHP: "8.3"},
			SupportsBootstrap: true,
		},
	}
	child := types.FrameworkSpec{
		Parent:     "parent",
		Definition: types.FrameworkDefinition{Name: "child"},
		Patch: types.FrameworkPatch{
			Config:            types.Set(engine.FrameworkConfig{Runtime: "node", DefaultNodeVer: "20"}),
			SupportsBootstrap: types.Clear[bool](),
		},
	}

	registry, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{root, child})
	if err != nil {
		t.Fatalf("NewRegistryFromSpecs() error = %v", err)
	}
	definition, ok := registry.Get("child")
	if !ok {
		t.Fatal("child definition was not resolved")
	}
	if definition.Config.Runtime != "node" || definition.Config.DefaultNodeVer != "20" {
		t.Errorf("Config = %#v, want child runtime override", definition.Config)
	}
	if definition.SupportsBootstrap {
		t.Error("SupportsBootstrap should be explicitly cleared")
	}
}

func TestDefaultRegistryResolvesMagentoFamilies(t *testing.T) {
	mageOS, ok := frameworks.Get("mageos")
	if !ok {
		t.Fatal("mageos definition was not registered")
	}
	if mageOS.Parent != "magento2" {
		t.Errorf("mageos Parent = %q, want magento2", mageOS.Parent)
	}
	if got, want := frameworks.Lineage("mageos"), []string{"magento2", "mageos"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lineage(mageos) = %v, want %v", got, want)
	}
	if !frameworks.IsA("mageos", "magento2") {
		t.Error("mageos should inherit Magento 2 behavior")
	}
	if mageOS.VersionProfileResolver != nil {
		t.Error("mageos must explicitly clear Magento 2's version-profile resolver")
	}

	openMage, ok := frameworks.Get("openmage")
	if !ok {
		t.Fatal("openmage definition was not registered")
	}
	if openMage.Parent != "magento1" {
		t.Errorf("openmage Parent = %q, want magento1", openMage.Parent)
	}
	if got, want := frameworks.Lineage("openmage"), []string{"magento1", "openmage"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lineage(openmage) = %v, want %v", got, want)
	}
	if !frameworks.IsA("openmage", "magento1") {
		t.Error("openmage should inherit Magento 1 behavior")
	}
}

func TestRegistryGetReturnsIndependentDefinitionSlices(t *testing.T) {
	registry, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{{
		Definition: types.FrameworkDefinition{
			Name:         "parent",
			Aliases:      []string{"base"},
			PHPStanPaths: []string{"app", "src"},
		},
	}})
	if err != nil {
		t.Fatalf("NewRegistryFromSpecs() error = %v", err)
	}

	first, ok := registry.Get("parent")
	if !ok {
		t.Fatal("parent definition was not resolved")
	}
	first.Aliases[0] = "changed-alias"
	first.PHPStanPaths[0] = "changed-path"

	second, ok := registry.Get("parent")
	if !ok {
		t.Fatal("parent definition was no longer present")
	}
	if got, want := second.Aliases, []string{"base"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Aliases after caller mutation = %v, want %v", got, want)
	}
	if got, want := second.PHPStanPaths, []string{"app", "src"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PHPStanPaths after caller mutation = %v, want %v", got, want)
	}
}

func TestRegistryBuildPreservesExplicitEmptyManifestSlices(t *testing.T) {
	registry, err := frameworks.NewRegistryFromSpecs([]types.FrameworkSpec{{
		Definition: types.FrameworkDefinition{
			Name: "empty-manifest",
			Manifest: engine.FrameworkManifestConfig{
				Sync: engine.FrameworkSyncConfig{
					MediaExcludes: engine.FrameworkMediaExcludeSet{NonAll: []string{}},
				},
			},
		},
	}})
	if err != nil {
		t.Fatalf("NewRegistryFromSpecs() error = %v", err)
	}

	definition, ok := registry.Get("empty-manifest")
	if !ok {
		t.Fatal("empty-manifest definition was not resolved")
	}
	if definition.Manifest.Sync.MediaExcludes.NonAll == nil {
		t.Fatal("explicit empty NonAll excludes became nil")
	}
	if len(definition.Manifest.Sync.MediaExcludes.NonAll) != 0 {
		t.Errorf("NonAll excludes = %v, want explicit empty slice", definition.Manifest.Sync.MediaExcludes.NonAll)
	}
}
