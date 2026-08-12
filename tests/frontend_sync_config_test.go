package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"gopkg.in/yaml.v3"
)

func TestFrontendSyncDefaultsToFalse(t *testing.T) {
	config := engine.Config{}

	if config.Stack.Features.FrontendSync {
		t.Fatal("expected frontend_sync to default to false")
	}
}

func TestFrontendSyncParsesFromYAML(t *testing.T) {
	var config engine.Config
	if err := yaml.Unmarshal([]byte("stack:\n  features:\n    frontend_sync: true\n"), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if !config.Stack.Features.FrontendSync {
		t.Fatal("expected frontend_sync to parse as true")
	}
}

func TestFrontendSyncRejectsLegacyLiveReloadYAML(t *testing.T) {
	var config engine.Config
	err := yaml.Unmarshal([]byte("stack:\n  features:\n    livereload: true\n"), &config)
	if err == nil {
		t.Fatal("expected legacy livereload configuration to be rejected")
	}
	if !strings.Contains(err.Error(), "livereload") {
		t.Fatalf("expected livereload validation error, got %v", err)
	}
}

func TestFrontendSyncValidationUsesRegisteredFrameworkCapability(t *testing.T) {
	const capableFramework = "synthetic-frontend-capable"
	engine.RegisterFrameworkCapabilities(capableFramework, engine.FrameworkCapabilities{FrontendSync: true})

	config := engine.Config{
		ProjectName: "frontend-sync",
		Domain:      "frontend-sync.test",
		Stack: engine.Stack{
			Features: engine.Features{FrontendSync: true},
			Services: engine.Services{WebServer: "nginx", Search: "none", Cache: "none", Queue: "none"},
		},
	}

	config.Framework = capableFramework
	if err := engine.ValidateConfig(config); err != nil {
		t.Fatalf("registered frontend-sync capability was rejected: %v", err)
	}

	config.Framework = "synthetic-frontend-incapable"
	err := engine.ValidateConfig(config)
	if err == nil || !strings.Contains(err.Error(), "frontend_sync") || !strings.Contains(err.Error(), config.Framework) {
		t.Fatalf("incapable framework validation error = %v", err)
	}
}

func TestFrameworkRegistryProjectsFrontendSyncCapabilityIntoEngine(t *testing.T) {
	var capable, incapable bool
	for _, definition := range frameworks.All() {
		want := definition.FrontendSyncRenderer != nil
		if got := engine.FrameworkSupportsFrontendSync(definition.Name); got != want {
			t.Fatalf("framework %q frontend-sync capability = %t, want %t", definition.Name, got, want)
		}
		capable = capable || want
		incapable = incapable || !want
	}
	if !capable || !incapable {
		t.Fatalf("registry fixture must exercise capable and incapable frameworks: capable=%t incapable=%t", capable, incapable)
	}
}
