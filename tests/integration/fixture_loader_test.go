//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateProjectFromFixture(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "fixture-copy")

	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected copied .govard.yml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "composer.json")); err != nil {
		t.Fatalf("expected copied composer.json: %v", err)
	}
}
