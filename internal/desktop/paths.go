package desktop

import (
	"fmt"
	"os"
	"path/filepath"
)

func FindRepoRoot() (string, error) {
	if override := os.Getenv("GOVARD_TEST_REPO_ROOT"); override != "" {
		return override, nil
	}

	start, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root, ok := findRootFrom(start); ok {
		return root, nil
	}

	if exe, err := os.Executable(); err == nil {
		if root, ok := findRootFrom(filepath.Dir(exe)); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("could not locate repository root from %s", start)
}

func exists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func findRootFrom(start string) (string, bool) {
	dir := start
	for {
		if exists(filepath.Join(dir, "go.mod")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
