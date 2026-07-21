package tests

import (
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

func TestMageOSDetectionViaProductPackage(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"mage-os/product-community-edition": "1.3.1",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "mageos" {
		t.Errorf("Expected framework mageos, got %s", metadata.Framework)
	}
	if metadata.Version != "1.3.1" {
		t.Errorf("Expected version 1.3.1, got %s", metadata.Version)
	}
}

func TestMageOSDetectionViaProjectPackage(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"mage-os/project-community-edition": "1.3.1",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "mageos" {
		t.Errorf("Expected framework mageos, got %s", metadata.Framework)
	}
}

func TestMageOSFrameworkConfigClonesMagento2Defaults(t *testing.T) {
	mageos, ok := engine.GetFrameworkConfig("mageos")
	if !ok {
		t.Fatal("expected mageos to have a FrameworkConfig entry")
	}
	magento2, ok := engine.GetFrameworkConfig("magento2")
	if !ok {
		t.Fatal("expected magento2 to have a FrameworkConfig entry")
	}

	if mageos.DatabaseName != "mageos" {
		t.Errorf("DatabaseName = %q, want %q", mageos.DatabaseName, "mageos")
	}
	if mageos.DefaultPHP != "8.4" {
		t.Errorf("DefaultPHP = %q, want %q", mageos.DefaultPHP, "8.4")
	}
	if mageos.NGINXTemplate != magento2.NGINXTemplate {
		t.Errorf("NGINXTemplate = %q, want to match magento2's %q", mageos.NGINXTemplate, magento2.NGINXTemplate)
	}
	if mageos.NGINXPUBLIC != magento2.NGINXPUBLIC {
		t.Errorf("NGINXPUBLIC = %q, want to match magento2's %q", mageos.NGINXPUBLIC, magento2.NGINXPUBLIC)
	}
	if len(mageos.Includes) != len(magento2.Includes) {
		t.Fatalf("Includes length = %d, want to match magento2's %d", len(mageos.Includes), len(magento2.Includes))
	}
	for i, inc := range magento2.Includes {
		if mageos.Includes[i] != inc {
			t.Errorf("Includes[%d] = %q, want %q (matching magento2)", i, mageos.Includes[i], inc)
		}
	}
}

func TestMageOSManifestClonesMagento2(t *testing.T) {
	mageos, ok := engine.GetFrameworkManifestConfig("mageos")
	if !ok {
		t.Fatal("expected mageos to have a manifest entry")
	}
	magento2, ok := engine.GetFrameworkManifestConfig("magento2")
	if !ok {
		t.Fatal("expected magento2 to have a manifest entry")
	}
	if len(mageos.Ignored) != len(magento2.Ignored) {
		t.Errorf("Ignored table count = %d, want to match magento2's %d", len(mageos.Ignored), len(magento2.Ignored))
	}
	if len(mageos.Sensitive) != len(magento2.Sensitive) {
		t.Errorf("Sensitive table count = %d, want to match magento2's %d", len(mageos.Sensitive), len(magento2.Sensitive))
	}
}

func TestMageOSRegistryDefinition(t *testing.T) {
	def, ok := frameworks.Get("mageos")
	if !ok {
		t.Fatal("expected mageos to be registered")
	}
	if def.Name != "mageos" {
		t.Errorf("Name = %q, want %q", def.Name, "mageos")
	}
	if def.Config.DefaultPHP != "8.4" {
		t.Errorf("registry Config.DefaultPHP = %q, want %q", def.Config.DefaultPHP, "8.4")
	}
}
