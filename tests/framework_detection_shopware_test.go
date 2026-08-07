package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestShopwareDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"shopware/core": "6.6.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "shopware" {
		t.Errorf("Expected framework shopware, got %s", metadata.Framework)
	}
}
