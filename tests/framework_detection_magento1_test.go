package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestMagento1Discovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"app/Mage.php": "",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1, got %s", metadata.Framework)
	}
}

func TestMagento1LocalXMLDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"app/etc/local.xml": "<config></config>",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1, got %s", metadata.Framework)
	}
}
