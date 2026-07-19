package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

type composerLockPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type composerLockFile struct {
	Packages    []composerLockPackage `json:"packages"`
	PackagesDev []composerLockPackage `json:"packages-dev"`
}

type composerInstalledV2 struct {
	Packages []composerLockPackage `json:"packages"`
}

// VendorSatisfiesComposerLock reports whether vendor/composer/installed.json
// under projectRoot already contains every package (name + exact version)
// listed in composer.lock. It is a pure, filesystem-only check used to skip
// re-running `composer install` when nothing has changed.
//
// This is an optimization check, not a critical flow: any missing or
// unparsable file yields (false, nil) rather than an error, so callers can
// always fall back to their existing "run composer install" path without
// special-casing an error return.
func VendorSatisfiesComposerLock(projectRoot string) (bool, error) {
	lockPackages, ok := readComposerLockPackages(projectRoot)
	if !ok || len(lockPackages) == 0 {
		return false, nil
	}

	installedPackages, ok := readInstalledPackages(projectRoot)
	if !ok {
		return false, nil
	}

	for name, version := range lockPackages {
		installedVersion, present := installedPackages[name]
		if !present || installedVersion != version {
			return false, nil
		}
	}
	return true, nil
}

func readComposerLockPackages(projectRoot string) (map[string]string, bool) {
	payload, err := os.ReadFile(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return nil, false
	}

	var lock composerLockFile
	if err := json.Unmarshal(payload, &lock); err != nil {
		return nil, false
	}

	all := append(append([]composerLockPackage{}, lock.Packages...), lock.PackagesDev...)
	packages := make(map[string]string, len(all))
	for _, pkg := range all {
		if pkg.Name == "" {
			continue
		}
		packages[pkg.Name] = pkg.Version
	}
	return packages, true
}

func readInstalledPackages(projectRoot string) (map[string]string, bool) {
	payload, err := os.ReadFile(filepath.Join(projectRoot, "vendor", "composer", "installed.json"))
	if err != nil {
		return nil, false
	}

	trimmed := bytes.TrimSpace(payload)
	var list []composerLockPackage
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Composer 1.x: installed.json is a bare JSON array of packages.
		if err := json.Unmarshal(payload, &list); err != nil {
			return nil, false
		}
	} else {
		// Composer 2.x: installed.json is an object with a "packages" key.
		var v2 composerInstalledV2
		if err := json.Unmarshal(payload, &v2); err != nil {
			return nil, false
		}
		list = v2.Packages
	}

	packages := make(map[string]string, len(list))
	for _, pkg := range list {
		if pkg.Name == "" {
			continue
		}
		packages[pkg.Name] = pkg.Version
	}
	return packages, true
}
