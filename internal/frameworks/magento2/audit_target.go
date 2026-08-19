package magento2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/frameworks/types"
)

// moduleDeclarationFile is the module declaration Magento reads to register an
// in-app module, relative to the module root.
const moduleDeclarationFile = "etc/module.xml"

var magentoAuditPackages = map[string]struct{}{
	"magento/framework":                  {},
	"magento/product-community-edition":  {},
	"magento/product-enterprise-edition": {},
	"magento/project-community-edition":  {},
	"magento/project-enterprise-edition": {},
	"mage-os/product-community-edition":  {},
	"mage-os/project-community-edition":  {},
}

type auditComposerManifest struct {
	Type    string                     `json:"type"`
	Require map[string]json.RawMessage `json:"require"`
}

// ResolveAuditTarget classifies a Magento project, a module nested inside one,
// or a standalone Magento module from a filesystem path.
func ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
	startPath, err := canonicalAuditPath(request.StartPath)
	if err != nil {
		return types.AuditTarget{}, false, err
	}

	moduleRoot, projectRoot, err := findMagentoAuditAncestors(startPath)
	if err != nil {
		return types.AuditTarget{}, false, err
	}
	hasMagentoEvidence := moduleRoot != "" || projectRoot != ""

	switch request.ModeOverride {
	case "", types.AuditTargetAuto:
		if moduleRoot != "" {
			if projectRoot != "" {
				return moduleAuditTarget(projectRoot, moduleRoot), true, nil
			}
			return standaloneAuditTarget(moduleRoot), true, nil
		}
		if projectRoot != "" {
			return projectAuditTarget(projectRoot), true, nil
		}
		return types.AuditTarget{}, false, nil
	case types.AuditTargetProject:
		if projectRoot != "" {
			return projectAuditTarget(projectRoot), true, nil
		}
	case types.AuditTargetModule:
		if moduleRoot != "" && projectRoot != "" {
			return moduleAuditTarget(projectRoot, moduleRoot), true, nil
		}
		if hasMagentoEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q requires a Magento module inside a Magento project", request.ModeOverride)
		}
		return types.AuditTarget{}, false, nil
	case types.AuditTargetStandalone:
		if moduleRoot != "" {
			return standaloneAuditTarget(moduleRoot), true, nil
		}
		if hasMagentoEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q requires a Magento module directory (etc/module.xml or a Composer package of type magento2-module)", request.ModeOverride)
		}
		return types.AuditTarget{}, false, nil
	default:
		if hasMagentoEvidence {
			return types.AuditTarget{}, true, fmt.Errorf("unknown audit target mode %q", request.ModeOverride)
		}
		return types.AuditTarget{}, false, nil
	}

	if hasMagentoEvidence {
		return types.AuditTarget{}, true, fmt.Errorf("audit target mode %q requires a Magento project", request.ModeOverride)
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

func findMagentoAuditAncestors(startPath string) (moduleRoot, projectRoot string, err error) {
	var packagedRoot, declaredRoot string
	for current := startPath; ; current = filepath.Dir(current) {
		if packagedRoot == "" {
			markers, moduleErr := classifyMagentoModule(current)
			if moduleErr != nil {
				return "", "", moduleErr
			}
			switch {
			case markers.composerPackage:
				packagedRoot = current
			case markers.declaration && declaredRoot == "":
				declaredRoot = current
			}
		}

		isProject, projectErr := isMagentoProject(current)
		if projectErr != nil {
			return "", "", projectErr
		}
		if isProject {
			return preferredModuleRoot(packagedRoot, declaredRoot), current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return preferredModuleRoot(packagedRoot, declaredRoot), "", nil
		}
	}
}

// preferredModuleRoot picks the module root the analysis should use. A Composer
// package boundary wins over a declaration-only directory nested inside it: the
// package root is what carries composer.json and composer.lock, so it is the
// only root at which dependencies can be resolved. Packages that keep their
// module code under src/ are exactly this shape, and rooting them at src/ would
// analyze them with no dependency symbols at all. A declaration-only root is
// used when no enclosing Composer module package exists anywhere in the
// ancestry, which is the app/code case.
func preferredModuleRoot(packagedRoot, declaredRoot string) string {
	if packagedRoot != "" {
		return packagedRoot
	}
	return declaredRoot
}

// magentoModuleMarkers records which module markers one directory carries. Both
// are independent structural signals: an etc/module.xml declaration is how
// Magento itself registers in-app modules, which usually ship no Composer
// manifest of their own, while a Composer manifest of type magento2-module is
// how separately distributed modules are packaged. A directory carrying either
// is a module root candidate; which candidate wins is decided by
// preferredModuleRoot.
type magentoModuleMarkers struct {
	composerPackage bool
	declaration     bool
}

func classifyMagentoModule(directory string) (magentoModuleMarkers, error) {
	declared, err := hasMagentoModuleDeclaration(directory)
	if err != nil {
		return magentoModuleMarkers{}, err
	}

	manifest, exists, err := readAuditComposerManifest(directory)
	if err != nil {
		return magentoModuleMarkers{}, err
	}
	return magentoModuleMarkers{
		composerPackage: exists && manifest.Type == "magento2-module",
		declaration:     declared,
	}, nil
}

func hasMagentoModuleDeclaration(directory string) (bool, error) {
	path := filepath.Join(directory, moduleDeclarationFile)
	marker, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect Magento module declaration %q: %w", path, err)
	}
	return marker.Mode().IsRegular(), nil
}

func isMagentoProject(directory string) (bool, error) {
	marker, err := os.Lstat(filepath.Join(directory, BinMagento))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect Magento CLI marker in %q: %w", directory, err)
	}
	if !marker.Mode().IsRegular() {
		return false, nil
	}

	manifest, exists, err := readAuditComposerManifest(directory)
	if err != nil || !exists {
		return false, err
	}
	for packageName := range manifest.Require {
		if _, ok := magentoAuditPackages[packageName]; ok {
			return true, nil
		}
	}
	return false, nil
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
		Framework:   "magento2",
		ProjectRoot: projectRoot,
		TargetPath:  projectRoot,
		Mode:        types.AuditTargetProject,
	}
}

func moduleAuditTarget(projectRoot, moduleRoot string) types.AuditTarget {
	return types.AuditTarget{
		Framework:   "magento2",
		ProjectRoot: projectRoot,
		TargetPath:  moduleRoot,
		Mode:        types.AuditTargetModule,
	}
}

func standaloneAuditTarget(moduleRoot string) types.AuditTarget {
	return types.AuditTarget{
		Framework:  "magento2",
		TargetPath: moduleRoot,
		Mode:       types.AuditTargetStandalone,
	}
}
