package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MediaGuardFinding describes one PHP file found inside pub/media.
type MediaGuardFinding struct {
	Path string `json:"path"`
}

// MediaGuardResult is the outcome of scanning pub/media for executable uploads.
type MediaGuardResult struct {
	Status   string              `json:"status"` // "passed" or "failed"
	Findings []MediaGuardFinding `json:"findings"`
}

// ScanMediaGuard walks projectRoot/pub/media and returns every *.php/*.phtml/*.pht file.
// The scan is name-only, mirroring the glint media-guard phase (milliseconds even on big trees).
func ScanMediaGuard(projectRoot string) []MediaGuardFinding {
	mediaDir := filepath.Join(projectRoot, "pub", "media")
	info, err := os.Stat(mediaDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	var findings []MediaGuardFinding
	_ = filepath.WalkDir(mediaDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".php") || strings.HasSuffix(name, ".phtml") || strings.HasSuffix(name, ".pht") {
			rel, relErr := filepath.Rel(projectRoot, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)
			findings = append(findings, MediaGuardFinding{Path: rel})
		}
		return nil
	})
	return findings
}

// RunMediaGuard returns a passed/failed result for projectRoot's pub/media.
func RunMediaGuard(projectRoot string) MediaGuardResult {
	findings := ScanMediaGuard(projectRoot)
	if len(findings) > 0 {
		return MediaGuardResult{Status: "failed", Findings: findings}
	}
	return MediaGuardResult{Status: "passed"}
}
