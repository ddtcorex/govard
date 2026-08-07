package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestEmdashDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"package.json": packageJSON(t, map[string]string{
			"astro":  "^6.1.2",
			"emdash": "^0.1.0",
		}),
		"astro.config.mjs": `import emdash from "emdash/astro";`,
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "emdash" {
		t.Errorf("Expected framework emdash, got %s", metadata.Framework)
	}
	if metadata.Version != "^0.1.0" {
		t.Errorf("Expected version ^0.1.0, got %s", metadata.Version)
	}
}
