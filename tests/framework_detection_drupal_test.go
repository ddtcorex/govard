package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestDrupalDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"drupal/core": "10.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "drupal" {
		t.Errorf("Expected framework drupal, got %s", metadata.Framework)
	}
}
