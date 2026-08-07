package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestLaravelDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"laravel/framework": "11.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "laravel" {
		t.Errorf("Expected framework laravel, got %s", metadata.Framework)
	}
	if metadata.Version != "11.0.0" {
		t.Errorf("Expected version 11.0.0, got %s", metadata.Version)
	}
}
