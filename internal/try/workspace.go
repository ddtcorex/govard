// Package try holds the logic behind `govard try`: creating a throwaway
// workspace, choosing the origin to probe, and waiting for it to answer.
// It is a library package on purpose, so the command stays thin and the
// behaviour can be tested without a shell.
package try

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// timestampLayout keeps workspace names sortable and unique per second.
const timestampLayout = "20060102-150405"

// maxWorkspaceSuffix bounds the collision search so a broken clock or a
// permission problem surfaces as an error instead of an endless loop.
const maxWorkspaceSuffix = 99

// CreateWorkspace creates a fresh trial directory under parent named
// <framework>-<timestamp>, adding -2, -3 and so on when that name is already
// taken. It returns the absolute path of the created directory.
func CreateWorkspace(parent, framework string, now time.Time) (string, error) {
	base := filepath.Join(parent, fmt.Sprintf("%s-%s", framework, now.Format(timestampLayout)))

	for suffix := 1; suffix <= maxWorkspaceSuffix; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}

		err := os.Mkdir(candidate, 0o700)
		if err == nil {
			abs, absErr := filepath.Abs(candidate)
			if absErr != nil {
				return "", fmt.Errorf("create try workspace: %w", absErr)
			}
			return abs, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("create try workspace: %w", err)
		}
	}

	return "", fmt.Errorf("create try workspace: no free name for %s", base)
}

// PrepareDir creates path when it does not exist and refuses a directory that
// already holds files, so an explicit --dir can never be emptied by accident.
func PrepareDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if mkErr := os.MkdirAll(path, 0o700); mkErr != nil {
			return fmt.Errorf("prepare try workspace: %w", mkErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("prepare try workspace: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("prepare try workspace: %s exists and is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("prepare try workspace: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("prepare try workspace: %s is not empty", path)
	}

	return nil
}
