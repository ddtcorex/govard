package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestMagentoDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"magento/product-community-edition": "2.4.7",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento2" {
		t.Errorf("Expected framework magento2, got %s", metadata.Framework)
	}
}

func TestMagento2AuthJSONDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"auth.json": `{"http-basic":{"repo.magento.com":{"username":"u","password":"p"}}}`,
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento2" {
		t.Errorf("Expected framework magento2, got %s", metadata.Framework)
	}
}
