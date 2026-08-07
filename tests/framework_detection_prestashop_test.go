package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestPrestaShopDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"config/defines.inc.php": "<?php\ndefine('_PS_VERSION_', '8.1.5');\n",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "prestashop" {
		t.Errorf("Expected framework prestashop, got %s", metadata.Framework)
	}
}
