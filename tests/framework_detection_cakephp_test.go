package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestCakephpDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"cakephp/cakephp": "5.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "cakephp" {
		t.Errorf("Expected framework cakephp, got %s", metadata.Framework)
	}
}
