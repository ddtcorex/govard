//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectFixtureExists(t *testing.T) {
	env := NewTestEnvironment(t)

	required := []string{
		"tests/integration/projects/magento2/options-local/govard.yml",
		"tests/integration/projects/magento2/options-dev/govard.yml",
		"tests/integration/projects/magento2/options-staging/govard.yml",
	}

	for _, rel := range required {
		path := filepath.Join(env.ProjectRoot, rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected fixture file %s: %v", path, err)
		}
	}
}
