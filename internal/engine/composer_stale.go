package engine

import (
	"os"
	"path/filepath"
)

// IsComposerStale reports whether the Composer install is stale for projectRoot.
// It returns true when composer.lock exists but vendor/composer/installed.json
// does not satisfy it, or when composer.json is newer than composer.lock.
func IsComposerStale(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	lockPath := filepath.Join(projectRoot, "composer.lock")
	jsonPath := filepath.Join(projectRoot, "composer.json")
	// If lock is missing but json exists, consider stale (needs install).
	if _, err := os.Stat(lockPath); err != nil {
		if _, jsonErr := os.Stat(jsonPath); jsonErr == nil {
			return true
		}
		return false
	}
	// mtime check: json newer than lock -> stale
	if lockInfo, err := os.Stat(lockPath); err == nil {
		if jsonInfo, err := os.Stat(jsonPath); err == nil {
			if jsonInfo.ModTime().After(lockInfo.ModTime()) {
				return true
			}
		}
	}
	// vendor vs lock check
	if ok, _ := VendorSatisfiesComposerLock(projectRoot); !ok {
		// Only report stale if lock exists and vendor is expected (i.e. json exists)
		if _, err := os.Stat(jsonPath); err == nil {
			return true
		}
	}
	return false
}

// ComposerStaleWarning returns a human-readable warning when Composer is stale, or empty.
func ComposerStaleWarning(projectRoot string) string {
	if IsComposerStale(projectRoot) {
		return "composer install stale (composer.lock out of date or vendor mismatch), run composer install"
	}
	return ""
}
