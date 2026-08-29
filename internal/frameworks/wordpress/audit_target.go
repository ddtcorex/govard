package wordpress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/frameworks/types"
)

var wordPressAuditPackages = map[string]struct{}{
	"johnpbloch/wordpress": {},
	"roots/wordpress":      {},
	"wordpress/wordpress":  {},
}

// ResolveAuditTarget detects a WordPress project for audit routing.
func ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
	startPath, err := canonicalAuditPath(request.StartPath)
	if err != nil {
		return types.AuditTarget{}, false, err
	}

	projectRoot, err := findWordPressAuditProject(startPath)
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
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q is not supported for framework %q: use --mode project (govard audit lint for this framework only supports project scope)", request.ModeOverride, "wordpress")
		}
		return types.AuditTarget{}, false, nil
	case types.AuditTargetStandalone:
		if hasEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q is not supported for framework %q: use --mode project (govard audit lint for this framework only supports project scope)", request.ModeOverride, "wordpress")
		}
		return types.AuditTarget{}, false, nil
	default:
		if hasEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("unknown audit target mode %q", request.ModeOverride)
		}
		return types.AuditTarget{}, false, nil
	}

	if hasEvidence {
		return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q requires a WordPress project", request.ModeOverride)
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

func findWordPressAuditProject(startPath string) (string, error) {
	for current := startPath; ; current = filepath.Dir(current) {
		isProject, err := isWordPressProject(current)
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

func isWordPressProject(directory string) (bool, error) {
	markers := []string{
		filepath.Join(directory, "wp-config.php"),
		filepath.Join(directory, "wp-includes", "version.php"),
		filepath.Join(directory, "web", "wp", "wp-includes", "version.php"),
	}
	for _, markerPath := range markers {
		marker, err := os.Lstat(markerPath)
		if err == nil {
			if marker.Mode().IsRegular() {
				return true, nil
			}
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("inspect WordPress marker in %q: %w", directory, err)
		}
	}
	manifest, exists, err := readAuditComposerManifest(directory)
	if err != nil || !exists {
		return false, err
	}
	for pkg := range manifest.Require {
		if _, ok := wordPressAuditPackages[pkg]; ok {
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
		Framework:   "wordpress",
		ProjectRoot: projectRoot,
		TargetPath:  projectRoot,
		Mode:        types.AuditTargetProject,
	}
}
