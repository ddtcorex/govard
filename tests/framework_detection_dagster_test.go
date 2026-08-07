package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestDagsterDiscoveryViaWorkspaceYAML(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"workspace.yaml": "load_from:\n  - python_module: my_pipeline\n",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "dagster" {
		t.Errorf("Expected framework dagster, got %s", metadata.Framework)
	}
}

func TestDagsterDiscoveryViaDagsterYAML(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"dagster.yaml": "storage:\n  sqlite:\n",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "dagster" {
		t.Errorf("Expected framework dagster, got %s", metadata.Framework)
	}
}
