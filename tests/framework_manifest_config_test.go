package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestGetFrameworkManifestConfigKnownFramework(t *testing.T) {
	config, ok := engine.GetFrameworkManifestConfig("magento2")
	if !ok {
		t.Fatal("expected magento2 to have a manifest config")
	}
	if len(config.Ignored) == 0 && len(config.Sensitive) == 0 {
		t.Error("expected magento2's manifest config to have at least one Ignored or Sensitive entry")
	}
}

func TestGetFrameworkManifestConfigUnknownFramework(t *testing.T) {
	if _, ok := engine.GetFrameworkManifestConfig("no-such-framework"); ok {
		t.Error("expected unknown framework to return ok=false")
	}
}
