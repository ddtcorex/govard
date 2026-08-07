package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestWordpressDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"johnpbloch/wordpress": "6.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "wordpress" {
		t.Errorf("Expected framework wordpress, got %s", metadata.Framework)
	}
}
