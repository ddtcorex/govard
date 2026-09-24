package tests

import (
	"testing"

	"govard/internal/frameworks/magento2"
)

// A table listed twice is harmless to the dump but hides a copy-paste slip: the
// entry meant to go there is usually the one that is missing.
func TestMagento2ManifestTableListsHaveNoDuplicates(t *testing.T) {
	for name, tables := range map[string][]string{
		"Ignored":   magento2.Manifest.Ignored,
		"Sensitive": magento2.Manifest.Sensitive,
	} {
		seen := map[string]bool{}
		for _, table := range tables {
			if seen[table] {
				t.Errorf("%s lists %q more than once", name, table)
			}
			seen[table] = true
		}
	}
}
