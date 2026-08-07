package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestDjangoDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"manage.py": "#!/usr/bin/env python\nimport django\n",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "django" {
		t.Errorf("Expected framework django, got %s", metadata.Framework)
	}
}
