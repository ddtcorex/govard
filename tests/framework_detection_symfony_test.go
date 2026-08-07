package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestSymfonyDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"symfony/framework-bundle": "7.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "symfony" {
		t.Errorf("Expected framework symfony, got %s", metadata.Framework)
	}
}
