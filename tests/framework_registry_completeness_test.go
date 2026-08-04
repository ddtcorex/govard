package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"
)

// TestRegistryIncludesSnapshotFrameworksAndCustom confirms the package-level
// registry contains every golden-snapshot framework plus Custom, which is a
// selectable escape hatch deliberately excluded from those snapshots.
func TestRegistryIncludesSnapshotFrameworksAndCustom(t *testing.T) {
	all := frameworks.All()
	if len(all) != len(allFrameworkNames)+1 {
		t.Fatalf("registry has %d frameworks, want %d snapshot frameworks plus custom", len(all), len(allFrameworkNames))
	}

	registered := map[string]bool{}
	for _, def := range all {
		registered[def.Name] = true
	}
	for _, name := range allFrameworkNames {
		if !registered[name] {
			t.Errorf("framework %q is in allFrameworkNames but not registered in internal/frameworks", name)
		}
	}
	if !registered["custom"] {
		t.Error("custom must be registered because CLI and desktop expose it as a selectable framework")
	}
}

// TestRegistryConfigMatchesEngine cross-checks the registry's Config field
// against the still-authoritative engine.GetFrameworkConfig for every
// framework - proving the registry reproduces existing behavior exactly,
// per the plan's core purpose.
func TestRegistryConfigMatchesEngine(t *testing.T) {
	for _, name := range allFrameworkNames {
		name := name
		t.Run(name, func(t *testing.T) {
			def, ok := frameworks.Get(name)
			if !ok {
				t.Fatalf("frameworks.Get(%q) returned ok=false", name)
			}
			want, ok := engine.GetFrameworkConfig(name)
			if !ok {
				t.Fatalf("engine.GetFrameworkConfig(%q) returned ok=false", name)
			}
			// engine.FrameworkConfig contains a []string field (Includes),
			// which makes the struct non-comparable with == / != - compare
			// via JSON instead, same technique as the Manifest check below.
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(def.Config)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("registry Config for %q does not match engine.GetFrameworkConfig:\nregistry: %s\nengine:   %s", name, gotJSON, wantJSON)
			}
		})
	}
}

// TestRegistryManifestMatchesEngine cross-checks the registry's Manifest
// field against engine.GetFrameworkManifestConfig for every framework.
func TestRegistryManifestMatchesEngine(t *testing.T) {
	for _, name := range allFrameworkNames {
		name := name
		t.Run(name, func(t *testing.T) {
			def, ok := frameworks.Get(name)
			if !ok {
				t.Fatalf("frameworks.Get(%q) returned ok=false", name)
			}
			want, ok := engine.GetFrameworkManifestConfig(name)
			if !ok {
				t.Fatalf("engine.GetFrameworkManifestConfig(%q) returned ok=false", name)
			}
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(def.Manifest)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("registry Manifest for %q does not match engine.GetFrameworkManifestConfig:\nregistry: %s\nengine:   %s", name, gotJSON, wantJSON)
			}
		})
	}
}

// TestRegistryBootstrapMatchesGoldenSnapshot cross-checks the registry's
// Bootstrap factory output against Plan 1's committed golden fixtures
// (tests/testdata/framework_snapshots/<framework>/bootstrap_fresh_commands.json),
// for every framework except magento2 and mageos. Their Bootstrap factory
// (magento2.NewBootstrap/mageos.NewBootstrap) is populated - not nil -
// but its FreshCommands() is a summary string, not what FreshInstall
// actually runs (see internal/frameworks/magento2/bootstrap.go and
// internal/frameworks/{magento2,mageos}/freshinstall.go for the real
// orchestration); this skip predates that
// distinction being real for any framework and applies to magento2/mageos
// specifically because no other framework's registered FreshCommands()
// diverges this much from its FreshInstall behavior.
func TestRegistryBootstrapMatchesGoldenSnapshot(t *testing.T) {
	goldenRoot := testDataDir(t)

	for _, name := range allFrameworkNames {
		if name == "magento2" || name == "mageos" {
			continue
		}
		name := name
		t.Run(name, func(t *testing.T) {
			def, ok := frameworks.Get(name)
			if !ok {
				t.Fatalf("frameworks.Get(%q) returned ok=false", name)
			}
			if def.Bootstrap == nil {
				t.Fatalf("%s Bootstrap should not be nil", name)
			}

			cmds := def.Bootstrap(bootstrap.Options{}).FreshCommands()
			gotJSON, err := json.MarshalIndent(cmds, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal fresh commands for %s: %v", name, err)
			}

			goldenPath := filepath.Join(goldenRoot, name, "bootstrap_fresh_commands.json")
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}
			if string(want) != string(gotJSON) {
				t.Errorf("registry Bootstrap output for %q does not match golden snapshot %s:\nregistry: %s\ngolden:   %s", name, goldenPath, gotJSON, want)
			}
		})
	}
}
