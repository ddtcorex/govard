package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestNextjsDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"package.json": packageJSON(t, map[string]string{
			"next": "14.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "nextjs" {
		t.Errorf("Expected framework nextjs, got %s", metadata.Framework)
	}
	if metadata.Version != "14.0.0" {
		t.Errorf("Expected version 14.0.0, got %s", metadata.Version)
	}
}

func TestNextjsDiscoveryWithMalformedComposerFallback(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": `{"name":"broken","require":{invalid json`,
		"package.json": packageJSON(t, map[string]string{
			"next": "14.2.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "nextjs" {
		t.Errorf("Expected framework nextjs, got %s", metadata.Framework)
	}
	if metadata.Version != "14.2.0" {
		t.Errorf("Expected version 14.2.0, got %s", metadata.Version)
	}
}
