package tests

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The desktop icon font is subset at build time (see
// desktop/frontend/scripts/subset-icons.mjs). These tests pin the two
// properties that make that safe: every ligature the source uses is in the
// shipped subset, and the shipped subset stays small.

// ligaturePattern matches Material Symbols ligature usages:
// <span class="material-symbols-outlined">settings</span> (and .tsx variants).
var iconLigaturePattern = regexp.MustCompile(`material-symbols-outlined[^>]*>\s*([a-z][a-z0-9_]+)\s*<`)

func repoIconSourceFiles(t *testing.T) []string {
	t.Helper()
	roots := []string{
		"desktop/frontend/modules",
		"desktop/frontend/islands",
		"desktop/frontend/ui",
		"desktop/frontend/services",
		"desktop/frontend/state",
	}
	var files []string
	for _, root := range roots {
		err := filepath.WalkDir(filepath.Join("..", root), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".js" && ext != ".tsx" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	files = append(files,
		filepath.Join("..", "desktop/frontend/main.js"),
		filepath.Join("..", "desktop/frontend/index.html"),
		filepath.Join("..", "desktop/frontend/preview.html"),
	)
	sort.Strings(files)
	return files
}

func sourceIconNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, path := range repoIconSourceFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range iconLigaturePattern.FindAllStringSubmatch(string(data), -1) {
			names[m[1]] = true
		}
	}
	return names
}

func subsetManifestNames(t *testing.T) map[string]bool {
	t.Helper()
	raw := readRepoFile(t, "desktop/frontend/assets/material-symbols.subset.json")
	var manifest struct {
		Icons []string `json:"icons"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("decode subset manifest: %v", err)
	}
	names := map[string]bool{}
	for _, name := range manifest.Icons {
		names[strings.TrimSpace(name)] = true
	}
	return names
}

func TestIconSubsetManifestCoversSources(t *testing.T) {
	manifest := subsetManifestNames(t)
	if len(manifest) == 0 {
		t.Fatal("subset manifest names no icons")
	}
	var missing []string
	for name := range sourceIconNames(t) {
		if !manifest[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("icons used in source but missing from the subset manifest: %s", strings.Join(missing, ", "))
	}
}

func TestIconSubsetSizeCeiling(t *testing.T) {
	info, err := os.Stat(filepath.Join("..", "desktop/frontend/assets/material-symbols.woff2"))
	if err != nil {
		t.Fatalf("stat subset font: %v", err)
	}
	const ceiling = 256 * 1024
	if info.Size() >= ceiling {
		t.Fatalf("subset font is %d bytes, must stay under %d", info.Size(), ceiling)
	}
}
