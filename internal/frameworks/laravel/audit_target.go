package laravel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/frameworks/types"
)

var laravelAuditPackages = map[string]struct{}{
	"laravel/framework": {},
}

// ResolveAuditTarget detects a Laravel project for audit routing. Laravel
// currently has no Govard-native lint profile (AuditLint is nil), so the
// resolver exists only to turn the generic "no framework can resolve audit
// target" into a helpful framework-specific message via the audit command's
// follow-up AuditLint-nil check.
func ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
	startPath, err := canonicalAuditPath(request.StartPath)
	if err != nil {
		return types.AuditTarget{}, false, err
	}

	projectRoot, err := findLaravelAuditProject(startPath)
	if err != nil {
		return types.AuditTarget{}, false, err
	}
	hasEvidence := projectRoot != ""

	switch request.ModeOverride {
	case "", types.AuditTargetAuto:
		if projectRoot != "" {
			return projectAuditTarget(projectRoot), true, nil
		}
		return types.AuditTarget{}, false, nil
	case types.AuditTargetProject:
		if projectRoot != "" {
			return projectAuditTarget(projectRoot), true, nil
		}
	case types.AuditTargetModule:
		if hasEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q is not supported for framework %q: laravel audit lint is not yet implemented (use govard tool php vendor/bin/pint or vendor/bin/phpstan directly)", request.ModeOverride, "laravel")
		}
		return types.AuditTarget{}, false, nil
	case types.AuditTargetStandalone:
		if hasEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q is not supported for framework %q: laravel audit lint is not yet implemented (use govard tool php vendor/bin/pint or vendor/bin/phpstan directly)", request.ModeOverride, "laravel")
		}
		return types.AuditTarget{}, false, nil
	default:
		if hasEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("unknown audit target mode %q", request.ModeOverride)
		}
		return types.AuditTarget{}, false, nil
	}

	if hasEvidence {
		return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q requires a Laravel project", request.ModeOverride)
	}
	return types.AuditTarget{}, false, nil
}

func canonicalAuditPath(startPath string) (string, error) {
	if strings.TrimSpace(startPath) == "" {
		return "", fmt.Errorf("audit target path is required")
	}
	resolved, err := filepath.EvalSymlinks(startPath)
	if err != nil {
		return "", fmt.Errorf("resolve audit target path %q: %w", startPath, err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("make audit target path absolute: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat audit target path %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("audit target path %q is not a directory", absolute)
	}
	return absolute, nil
}

func findLaravelAuditProject(startPath string) (string, error) {
	for current := startPath; ; current = filepath.Dir(current) {
		isProject, err := isLaravelProject(current)
		if err != nil {
			return "", err
		}
		if isProject {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil
		}
	}
}

func isLaravelProject(directory string) (bool, error) {
	marker, err := os.Lstat(filepath.Join(directory, BinArtisan))
	hasArtisan := err == nil && marker.Mode().IsRegular()
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect Laravel artisan marker in %q: %w", directory, err)
	}
	manifest, exists, err := readAuditComposerManifest(directory)
	if err != nil || !exists {
		return false, err
	}
	for pkg := range manifest.Require {
		if _, ok := laravelAuditPackages[pkg]; ok {
			if hasArtisan {
				return true, nil
			}
			return true, nil
		}
	}
	return false, nil
}

type auditComposerManifest struct {
	Require map[string]json.RawMessage `json:"require"`
}

func readAuditComposerManifest(directory string) (auditComposerManifest, bool, error) {
	path := filepath.Join(directory, "composer.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return auditComposerManifest{}, false, nil
		}
		return auditComposerManifest{}, false, fmt.Errorf("read Composer manifest %q: %w", path, err)
	}
	var manifest auditComposerManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return auditComposerManifest{}, false, fmt.Errorf("parse Composer manifest %q: %w", path, err)
	}
	return manifest, true, nil
}

func projectAuditTarget(projectRoot string) types.AuditTarget {
	return types.AuditTarget{
		Framework:   "laravel",
		ProjectRoot: projectRoot,
		TargetPath:  projectRoot,
		Mode:        types.AuditTargetProject,
	}
}
