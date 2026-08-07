package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

// TestFrameworkDetectionPriority covers detection outcomes that depend on a
// relationship between two specific frameworks (priority ordering, package
// aliasing) rather than a single framework in isolation - each case name
// carries the framework pairing; the test function itself stays generic.
func TestFrameworkDetectionPriority(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name: "magento2_wins_over_mageos",
			files: map[string]string{
				"composer.json": composerJSON(t, map[string]string{
					"magento/product-community-edition": "2.4.8",
					"mage-os/project-community-edition": "1.3.1",
				}),
			},
			expected: "magento2",
		},
		{
			name: "emdash_retains_legacy_priority_over_nextjs",
			files: map[string]string{
				"package.json": packageJSON(t, map[string]string{
					"emdash": "^0.1.0",
					"next":   "15.0.0",
				}),
			},
			expected: "emdash",
		},
		// Surprising but current real behavior: openmage/magento-lts maps to
		// "magento1", not "openmage" - openmage has no detection heuristic
		// of its own. This test locks that in so it can't be "fixed" by
		// accident in a future change.
		{
			name: "openmage_package_aliases_to_magento1",
			files: map[string]string{
				"composer.json": composerJSON(t, map[string]string{
					"openmage/magento-lts": "20.0.0",
				}),
			},
			expected: "magento1",
		},
		{
			name: "magento_hackathon_package_aliases_to_magento1",
			files: map[string]string{
				"composer.json": composerJSON(t, map[string]string{
					"magento-hackathon/magento-composer-installer": "3.0.0",
				}),
			},
			expected: "magento1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			testDir := tempProject(t, tt.files)
			metadata := engine.DetectFramework(testDir)
			if metadata.Framework != tt.expected {
				t.Errorf("expected framework %s, got %s", tt.expected, metadata.Framework)
			}
		})
	}
}

func tempProject(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", rel, err)
		}
	}
	return dir
}

func composerJSON(t *testing.T, require map[string]string) string {
	t.Helper()
	payload := map[string]map[string]string{"require": require}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to build composer.json: %v", err)
	}
	return string(data)
}

func packageJSON(t *testing.T, deps map[string]string) string {
	t.Helper()
	payload := map[string]map[string]string{"dependencies": deps}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to build package.json: %v", err)
	}
	return string(data)
}
