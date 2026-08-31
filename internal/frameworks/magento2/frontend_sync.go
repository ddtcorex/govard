package magento2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const frontendSyncThemesRoot = "app/design/frontend"

var composeNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeComposeProjectName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = composeNameSanitizer.ReplaceAllString(name, "-")
	return strings.Trim(strings.ToLower(name), "-._")
}

// FrontendSyncMode identifies the project-owned frontend runtime to run.
type FrontendSyncMode string

const (
	// FrontendSyncModeHyva runs the BrowserSync script owned by a Hyva theme.
	FrontendSyncModeHyva FrontendSyncMode = "hyva"
	// FrontendSyncModeLuma runs the standard Magento Luma Grunt watcher.
	FrontendSyncModeLuma FrontendSyncMode = "luma"
)

// FrontendSyncRuntime is the independently runnable frontend development
// runtime discovered from a Magento project.
type FrontendSyncRuntime struct {
	Mode              FrontendSyncMode
	WorkingDir        string
	Command           string
	PackageLockHash   string
	PackageJSONHash   string
	NodeModulesVolume string
	Themes            []FrontendSyncTheme
	BrowserSync       FrontendSyncBrowserSync
}

// FrontendSyncTheme describes one discovered Hyva theme's runtime paths.
type FrontendSyncTheme struct {
	Vendor            string
	Theme             string
	TailwindDir       string
	CSSOutputDir      string
	PackageLockHash   string
	PackageJSONHash   string
	BrowserSyncScript bool
}

// FrontendSyncBrowserSync identifies the project-owned BrowserSync runtime.
type FrontendSyncBrowserSync struct {
	TailwindDir       string
	PackageLockHash   string
	PackageJSONHash   string
	NodeModulesVolume string
}

// DiscoverFrontendSyncRuntime discovers exactly one explicit, project-owned
// frontend development runtime. A Hyva project owns BrowserSync through one
// scripts.browser-sync entry; Luma owns the standard root Grunt runtime.
func DiscoverFrontendSyncRuntime(root string) (FrontendSyncRuntime, error) {
	themes, err := DiscoverFrontendSyncThemes(root)
	if err != nil {
		return FrontendSyncRuntime{}, err
	}

	hyvaCandidates := make([]FrontendSyncTheme, 0, 1)
	for _, theme := range themes {
		if theme.BrowserSyncScript {
			hyvaCandidates = append(hyvaCandidates, theme)
		}
	}

	lumaFiles, err := discoverFrontendSyncLumaFiles(root)
	if err != nil {
		return FrontendSyncRuntime{}, err
	}
	if len(hyvaCandidates) > 0 && lumaFiles.configured() {
		missingLock := ""
		if !lumaFiles.packageLock {
			missingLock = fmt.Sprintf("; the conflicting Luma setup also requires %s for npm ci, so generate it with npm install or remove the Luma Gruntfile.js/package.json setup", filepath.Join(root, "package-lock.json"))
		}
		return FrontendSyncRuntime{}, fmt.Errorf("frontend sync found both Hyva BrowserSync and Luma Grunt runtimes; remove one frontend runtime before starting frontend sync%s", missingLock)
	}

	if len(hyvaCandidates) > 1 {
		paths := make([]string, 0, len(hyvaCandidates))
		for _, candidate := range hyvaCandidates {
			paths = append(paths, filepath.ToSlash(filepath.Join(candidate.TailwindDir, "package.json")))
		}
		return FrontendSyncRuntime{}, fmt.Errorf("frontend sync found multiple Hyva Tailwind BrowserSync scripts (%s); keep scripts.browser-sync in exactly one theme package.json", strings.Join(paths, ", "))
	}
	if len(hyvaCandidates) == 1 {
		for _, theme := range themes {
			if strings.TrimSpace(theme.PackageLockHash) == "" {
				return FrontendSyncRuntime{}, fmt.Errorf("frontend sync theme %s requires %s for npm ci; generate the lockfile with npm install", filepath.ToSlash(filepath.Join(theme.Vendor, theme.Theme)), filepath.ToSlash(filepath.Join(theme.TailwindDir, "package-lock.json")))
			}
		}
		watchers := buildFrontendSyncWatchers(themes)
		browserSync, err := SelectFrontendSyncBrowserSync(themes, watchers)
		if err != nil {
			return FrontendSyncRuntime{}, err
		}
		return FrontendSyncRuntime{
			Mode:              FrontendSyncModeHyva,
			WorkingDir:        browserSync.TailwindDir,
			Command:           "npm run browser-sync",
			PackageLockHash:   browserSync.PackageLockHash,
			PackageJSONHash:   browserSync.PackageJSONHash,
			NodeModulesVolume: browserSync.NodeModulesVolume,
			Themes:            themes,
			BrowserSync:       browserSync,
		}, nil
	}

	if lumaFiles.configured() {
		return discoverFrontendSyncLumaRuntime(root)
	}

	return FrontendSyncRuntime{}, fmt.Errorf("frontend sync requires either exactly one Hyva Tailwind package.json with a scripts.browser-sync command or root Gruntfile.js and package.json for Luma")
}

type frontendSyncLumaFiles struct {
	gruntfile   bool
	packageJSON bool
	packageLock bool
}

func (files frontendSyncLumaFiles) configured() bool {
	return files.gruntfile && files.packageJSON
}

func discoverFrontendSyncLumaFiles(root string) (frontendSyncLumaFiles, error) {
	gruntfile, err := frontendSyncRegularFile(filepath.Join(root, "Gruntfile.js"))
	if err != nil {
		return frontendSyncLumaFiles{}, err
	}
	packageJSON, err := frontendSyncRegularFile(filepath.Join(root, "package.json"))
	if err != nil {
		return frontendSyncLumaFiles{}, err
	}
	packageLock, err := frontendSyncRegularFile(filepath.Join(root, "package-lock.json"))
	if err != nil {
		return frontendSyncLumaFiles{}, err
	}
	return frontendSyncLumaFiles{
		gruntfile:   gruntfile,
		packageJSON: packageJSON,
		packageLock: packageLock,
	}, nil
}

func discoverFrontendSyncLumaRuntime(root string) (FrontendSyncRuntime, error) {
	packageJSONPath := filepath.Join(root, "package.json")
	packageJSON, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return FrontendSyncRuntime{}, fmt.Errorf("read Luma package manifest %s: %w", packageJSONPath, err)
	}
	if err := validateFrontendSyncJSONObject(packageJSON, packageJSONPath, "Luma package manifest"); err != nil {
		return FrontendSyncRuntime{}, err
	}
	packageLockPath := filepath.Join(root, "package-lock.json")
	packageLock, err := os.ReadFile(packageLockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FrontendSyncRuntime{}, fmt.Errorf("luma frontend sync requires %s for npm ci; generate the lockfile with npm install", packageLockPath)
		}
		return FrontendSyncRuntime{}, fmt.Errorf("read Luma package lock %s: %w", packageLockPath, err)
	}
	if err := validateFrontendSyncJSONObject(packageLock, packageLockPath, "Luma package lock"); err != nil {
		return FrontendSyncRuntime{}, err
	}

	packageJSONSum := sha256.Sum256(packageJSON)
	packageLockSum := sha256.Sum256(packageLock)
	packageLockHash := hex.EncodeToString(packageLockSum[:])
	return FrontendSyncRuntime{
		Mode:              FrontendSyncModeLuma,
		WorkingDir:        ".",
		Command:           "npx grunt watch",
		PackageLockHash:   packageLockHash,
		PackageJSONHash:   hex.EncodeToString(packageJSONSum[:]),
		NodeModulesVolume: "frontend-sync-luma-node-modules-" + packageLockHash,
	}, nil
}

func frontendSyncRegularFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect frontend runtime file %s: %w", path, err)
	}
	return info.Mode().IsRegular(), nil
}

// SelectFrontendSyncBrowserSync requires exactly one discovered theme to own
// the browser-sync package script. This makes the runtime deterministic while
// leaving BrowserSync version and configuration to the project.
func SelectFrontendSyncBrowserSync(themes []FrontendSyncTheme, watchers []FrontendSyncWatcher) (FrontendSyncBrowserSync, error) {
	candidates := make([]FrontendSyncTheme, 0, 1)
	for _, theme := range themes {
		if theme.BrowserSyncScript {
			candidates = append(candidates, theme)
		}
	}
	if len(candidates) != 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, filepath.ToSlash(filepath.Join(candidate.TailwindDir, "package.json")))
		}
		if len(candidates) == 0 {
			return FrontendSyncBrowserSync{}, fmt.Errorf("frontend sync requires exactly one Hyva Tailwind package.json with a scripts.browser-sync command; add it to the theme that owns BrowserSync")
		}
		return FrontendSyncBrowserSync{}, fmt.Errorf("frontend sync found multiple Hyva Tailwind BrowserSync scripts (%s); keep scripts.browser-sync in exactly one theme package.json", strings.Join(paths, ", "))
	}
	for _, watcher := range watchers {
		if watcher.TailwindDir == candidates[0].TailwindDir {
			return FrontendSyncBrowserSync{
				TailwindDir:       watcher.TailwindDir,
				PackageLockHash:   watcher.PackageLockHash,
				PackageJSONHash:   candidates[0].PackageJSONHash,
				NodeModulesVolume: "frontend-sync-browser-sync-node-modules-" + watcher.PackageLockHash,
			}, nil
		}
	}
	return FrontendSyncBrowserSync{}, fmt.Errorf("frontend sync could not find a watcher for BrowserSync theme %s", candidates[0].TailwindDir)
}

// FrontendSyncWatcher describes one rendered Tailwind watcher service.
type FrontendSyncWatcher struct {
	ServiceName       string
	TailwindDir       string
	PackageLockHash   string
	NodeModulesVolume string
}

func buildFrontendSyncWatchers(themes []FrontendSyncTheme) []FrontendSyncWatcher {
	baseNames := make([]string, len(themes))
	nameCounts := make(map[string]int, len(themes))
	for i, theme := range themes {
		baseName := sanitizeComposeProjectName(theme.Vendor + "-" + theme.Theme)
		if baseName == "" {
			baseName = "theme-" + frontendSyncIdentityHash(theme)[:12]
		}
		baseNames[i] = baseName
		nameCounts[baseName]++
	}

	watchers := make([]FrontendSyncWatcher, 0, len(themes))
	usedNames := make(map[string]struct{}, len(themes))
	for i, theme := range themes {
		normalizedName := baseNames[i]
		generatedSuffix := nameCounts[normalizedName] > 1
		if generatedSuffix {
			normalizedName += "-" + frontendSyncIdentityHash(theme)[:12]
		}
		preferredName := normalizedName
		for attempt := 2; ; attempt++ {
			_, alreadyUsed := usedNames[normalizedName]
			collidesWithNaturalName := generatedSuffix && nameCounts[normalizedName] > 0
			if !alreadyUsed && !collidesWithNaturalName {
				break
			}
			normalizedName = fmt.Sprintf("%s-%d", preferredName, attempt)
		}
		usedNames[normalizedName] = struct{}{}
		lockHash := strings.TrimSpace(theme.PackageLockHash)
		if lockHash == "" {
			lockHash = "no-lock"
		}
		watchers = append(watchers, FrontendSyncWatcher{
			ServiceName:       "watch-" + normalizedName,
			TailwindDir:       theme.TailwindDir,
			PackageLockHash:   lockHash,
			NodeModulesVolume: "frontend-sync-" + normalizedName + "-node-modules-" + lockHash,
		})
	}
	return watchers
}

func frontendSyncIdentityHash(theme FrontendSyncTheme) string {
	sum := sha256.Sum256([]byte(theme.Vendor + "\x00" + theme.Theme))
	return hex.EncodeToString(sum[:])
}

// DiscoverFrontendSyncThemes finds Hyva Tailwind roots in deterministic
// vendor/theme order.
func DiscoverFrontendSyncThemes(root string) ([]FrontendSyncTheme, error) {
	themesRoot := filepath.Join(root, filepath.FromSlash(frontendSyncThemesRoot))
	vendors, err := os.ReadDir(themesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read frontend themes directory %s: %w", themesRoot, err)
	}

	themes := make([]FrontendSyncTheme, 0)
	for _, vendor := range vendors {
		if !vendor.IsDir() {
			continue
		}

		vendorDir := filepath.Join(themesRoot, vendor.Name())
		themeEntries, err := os.ReadDir(vendorDir)
		if err != nil {
			return nil, fmt.Errorf("read frontend theme vendor directory %s: %w", vendorDir, err)
		}
		for _, theme := range themeEntries {
			if !theme.IsDir() {
				continue
			}

			tailwindDir := filepath.Join(vendorDir, theme.Name(), "web", "tailwind")
			packageJSONPath := filepath.Join(tailwindDir, "package.json")
			info, err := os.Stat(packageJSONPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("inspect frontend theme package %s: %w", packageJSONPath, err)
			}
			if !info.Mode().IsRegular() {
				continue
			}

			packageJSON, err := os.ReadFile(packageJSONPath)
			if err != nil {
				return nil, fmt.Errorf("read frontend theme package %s: %w", packageJSONPath, err)
			}
			var manifest struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(packageJSON, &manifest); err != nil {
				return nil, fmt.Errorf("frontend theme package %s must contain a valid JSON object: %w", packageJSONPath, err)
			}
			packageJSONSum := sha256.Sum256(packageJSON)
			packageLockHash, err := frontendSyncPackageLockHash(filepath.Join(tailwindDir, "package-lock.json"))
			if err != nil {
				return nil, err
			}
			relThemeDir := filepath.Join(frontendSyncThemesRoot, vendor.Name(), theme.Name())
			themes = append(themes, FrontendSyncTheme{
				Vendor:            vendor.Name(),
				Theme:             theme.Name(),
				TailwindDir:       filepath.ToSlash(filepath.Join(relThemeDir, "web", "tailwind")),
				CSSOutputDir:      filepath.ToSlash(filepath.Join(relThemeDir, "web", "css")),
				PackageLockHash:   packageLockHash,
				PackageJSONHash:   hex.EncodeToString(packageJSONSum[:]),
				BrowserSyncScript: strings.TrimSpace(manifest.Scripts["browser-sync"]) != "",
			})
		}
	}

	sort.Slice(themes, func(i, j int) bool {
		if themes[i].Vendor == themes[j].Vendor {
			return themes[i].Theme < themes[j].Theme
		}
		return themes[i].Vendor < themes[j].Vendor
	})
	return themes, nil
}

func frontendSyncPackageLockHash(lockPath string) (string, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read frontend theme package lock %s: %w", lockPath, err)
	}
	if err := validateFrontendSyncJSONObject(data, lockPath, "frontend theme package lock"); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateFrontendSyncJSONObject(data []byte, path, description string) error {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil || value == nil {
		if err == nil {
			err = fmt.Errorf("top-level value is not an object")
		}
		return fmt.Errorf("%s %s must contain a valid JSON object for npm ci: %w", description, path, err)
	}
	return nil
}
