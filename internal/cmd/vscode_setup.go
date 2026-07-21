package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const xdebugLaunchConfigName = "Listen for Xdebug (Govard)"

const (
	extIntelephense = "bmewburn.vscode-intelephense-client"
	extPHPStan      = "sanderronde.phpstan-vscode"
	extPHPCSFixer   = "junstyle.php-cs-fixer"
	extPHPCS        = "shevaua.phpcs"
	extPHPUnit      = "recca0120.vscode-phpunit"
	extXdebug       = "xdebug.php-debug"
)

var vscodeSetupGlobal bool
var vscodeSetupYes bool

var vscodeSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Write or update VSCode settings to use this project's container instead of the host",
	Long: `Write (or merge into) the VSCode settings needed to run PHP tooling inside the
project's container instead of the host machine.

Without --global, run this from inside a project (or any subdirectory of one)
to write .vscode/settings.json (Intelephense PHP version, PHPStan path mapping,
a PHPUnit path mapping if vendor/bin/phpunit is present, and — if
vendor/bin/phpcs is present — a PHPCS coding standard) and .vscode/launch.json
(a "Listen for Xdebug" configuration). If vendor/bin/phpstan is present but the
project has no phpstan.neon/.dist config of its own, phpstan.options is set to
a --level=0 default instead — kept in .vscode/settings.json rather than
writing a phpstan.neon at the project root, which is normally git-tracked.

Uses the project's last-used profile (e.g. an upgrade profile pinning a newer
PHP version) if one is registered, so settings match what's actually running
rather than always the base .govard.yml.

With --global, updates VSCode's user settings.json once for every project:
creates the govard-php / govard-php-cs-fixer / govard-phpcs wrapper scripts
under ~/.govard/bin and points php.validate.executablePath, phpstan.binCommand,
php-cs-fixer.executablePath, phpcs.executablePath, and phpunit.command at them.

Existing keys in either file are preserved; only the keys this command manages
are added or overwritten. Note: settings.json is parsed as plain JSON, so any
comments in it will be dropped when rewritten.

For each setting group whose VSCode extension isn't installed, you'll be asked
whether to install it (via "code --install-extension") before that group is
skipped. Pass --yes to install everything missing without asking.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vscodeSetupGlobal {
			return runVSCodeSetupGlobal()
		}
		return runVSCodeSetupProject()
	},
}

func init() {
	vscodeSetupCmd.Flags().BoolVar(&vscodeSetupGlobal, "global", false, "Update VSCode's user settings.json instead of the current project")
	vscodeSetupCmd.Flags().BoolVar(&vscodeSetupYes, "yes", false, "Install any missing required VSCode extensions without asking")
}

func runVSCodeSetupProject() error {
	root, err := findProjectRootUpward()
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return fmt.Errorf("switch to project root %q: %w", root, err)
	}

	// loadConfigWithProfile("") resolves to the project's last-used profile
	// (e.g. an upgrade profile pinning a newer PHP version) if one is
	// registered, rather than always reading the base .govard.yml — so
	// settings match whichever profile is actually running right now.
	config := loadConfigWithProfile("")
	workdir := engine.ResolveFrameworkAppWorkdir(config.Framework)

	set := map[string]interface{}{}
	var unset []string

	if ensureVSCodeExtension(extIntelephense, "Intelephense PHP version") {
		if version := normalizeIntelephensePHPVersion(config.Stack.PHPVersion); version != "" {
			set["intelephense.environment.phpVersion"] = version
		}
	}

	if phpstanAvailable(root) && ensureVSCodeExtension(extPHPStan, "PHPStan path mapping") {
		set["phpstan.paths"] = map[string]interface{}{
			root: workdir,
		}
		if hasPHPStanConfig(root) {
			// The project has its own phpstan.neon/.dist — let the
			// extension's config-file auto-detection use it, and clear any
			// phpstan.options default a previous run may have set before
			// that config existed so it can't override the project's rules.
			unset = append(unset, "phpstan.options")
		} else {
			// No phpstan.neon/.dist of the project's own — fall back to
			// --level=0 and default paths via phpstan.options instead of
			// writing a phpstan.neon at the project root, which is normally
			// git-tracked and not ours to create.
			set["phpstan.options"] = phpstanDefaultOptions(config.Framework)
			pterm.Info.Println("No phpstan.neon/.dist found — using phpstan.options (level 0) instead of writing a project file")
		}
	}

	if phpcsAvailable(root) && ensureVSCodeExtension(extPHPCS, "PHPCS standard") {
		// autoConfigSearch would otherwise let the extension auto-detect a
		// phpcs.xml/.dist ruleset and pass its *host* absolute path as
		// --standard, which the container can't read.
		set["phpcs.autoConfigSearch"] = false
		set["phpcs.standard"] = detectPHPCSStandard(root)
	}

	if phpunitAvailable(root) && ensureVSCodeExtension(extPHPUnit, "PHPUnit path mapping") {
		set["phpunit.paths"] = map[string]interface{}{
			root: workdir,
		}
	}

	if len(set) > 0 || len(unset) > 0 {
		settingsPath := filepath.Join(root, ".vscode", "settings.json")
		if err := mergeJSONObjectFile(settingsPath, set, unset); err != nil {
			return fmt.Errorf("write %s: %w", settingsPath, err)
		}
		pterm.Success.Printf("Updated %s\n", settingsPath)
	}

	if ensureVSCodeExtension(extXdebug, "launch.json Xdebug configuration") {
		launchPath := filepath.Join(root, ".vscode", "launch.json")
		if err := mergeLaunchConfig(launchPath, workdir); err != nil {
			return fmt.Errorf("write %s: %w", launchPath, err)
		}
		pterm.Success.Printf("Updated %s\n", launchPath)
	}

	return nil
}

func runVSCodeSetupGlobal() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home directory: %w", err)
	}

	binDir := filepath.Join(home, ".govard", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", binDir, err)
	}

	// php.validate.executablePath targets a VSCode built-in feature, not a
	// marketplace extension, so it's always wired.
	phpWrapper := filepath.Join(binDir, "govard-php")
	if err := ensureWrapperScript(phpWrapper, "php"); err != nil {
		return err
	}
	set := map[string]interface{}{
		"php.validate.executablePath": phpWrapper,
	}
	var unset []string
	var wrapperPaths []string

	if ensureVSCodeExtension(extPHPStan, "phpstan.binCommand") {
		set["phpstan.binCommand"] = []string{"govard", "vscode", "phpstan"}
		unset = append(unset, "phpstan.binPath")
	}

	if ensureVSCodeExtension(extPHPCSFixer, "php-cs-fixer.executablePath") {
		csFixerWrapper := filepath.Join(binDir, "govard-php-cs-fixer")
		if err := ensureWrapperScript(csFixerWrapper, "php-cs-fixer"); err != nil {
			return err
		}
		set["php-cs-fixer.executablePath"] = csFixerWrapper
		wrapperPaths = append(wrapperPaths, csFixerWrapper)
	}

	if ensureVSCodeExtension(extPHPCS, "phpcs.executablePath") {
		phpcsWrapper := filepath.Join(binDir, "govard-phpcs")
		if err := ensureWrapperScript(phpcsWrapper, "phpcs"); err != nil {
			return err
		}
		set["phpcs.executablePath"] = phpcsWrapper
		wrapperPaths = append(wrapperPaths, phpcsWrapper)
	}

	if ensureVSCodeExtension(extPHPUnit, "phpunit.command") {
		// phpunit.command is a template the extension tokenizes itself
		// (quote-aware, like a shell command line), so no wrapper script is
		// needed — "govard vscode phpunit" already prepends memory_limit=-1
		// and vendor/bin/phpunit; ${phpunitargs} supplies the actual CLI args.
		set["phpunit.command"] = "govard vscode phpunit ${phpunitargs}"
	}

	pterm.Success.Printf("Wrote %s%s\n", phpWrapper, joinWithLeadingComma(wrapperPaths))

	settingsPath, err := vscodeGlobalSettingsPath()
	if err != nil {
		return err
	}
	if err := mergeJSONObjectFile(settingsPath, set, unset); err != nil {
		return fmt.Errorf("write %s: %w", settingsPath, err)
	}
	pterm.Success.Printf("Updated %s\n", settingsPath)

	return nil
}

// ensureVSCodeExtension reports whether extensionID is installed. If it
// isn't, it warns which setting group needs it and — unless --yes was passed,
// in which case it installs unprompted — asks whether to install it now via
// `code --install-extension`. Returns true if the extension is (now)
// installed, so the caller can wire up settingLabel in this same run.
func ensureVSCodeExtension(extensionID, settingLabel string) bool {
	if isVSCodeExtensionInstalled(extensionID) {
		return true
	}

	pterm.Warning.Printf("%s needs the VSCode extension %s, which isn't installed.\n", settingLabel, extensionID)

	install := vscodeSetupYes
	if !install && stdinIsTerminal() {
		confirmed, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(true).
			Show(fmt.Sprintf("Install %s now?", extensionID))
		install = err == nil && confirmed
	}
	if !install {
		pterm.Info.Printf("Skipping %s. Re-run with --yes to install missing extensions automatically.\n", settingLabel)
		return false
	}

	if _, err := exec.LookPath("code"); err != nil {
		pterm.Error.Printf("Cannot install %s: the \"code\" CLI isn't on PATH. Install it manually, then re-run setup.\n", extensionID)
		return false
	}

	pterm.Info.Printf("Installing %s...\n", extensionID)
	installCmd := exec.Command("code", "--install-extension", extensionID)
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		pterm.Error.Printf("Failed to install %s: %v\n", extensionID, err)
		return false
	}

	if isVSCodeExtensionInstalled(extensionID) {
		return true
	}

	// `code --install-extension` reported success but didn't register in
	// extensions.json — this happens when it can't reach an already-running
	// VSCode window's live state (e.g. run from an external/sandboxed
	// process). The files may exist on disk without VSCode ever loading them.
	pterm.Warning.Printf(
		"%s reported success but %s isn't in VSCode's extensions manifest yet. "+
			"If you have VSCode open, install it from the Extensions panel (Ctrl+Shift+X) instead, then re-run setup.\n",
		extensionID, extensionID,
	)
	return false
}

// joinWithLeadingComma formats extra wrapper paths for the "Wrote ..." success
// message, e.g. ", /a, and /b" or "" if empty.
func joinWithLeadingComma(paths []string) string {
	switch len(paths) {
	case 0:
		return ""
	case 1:
		return " and " + paths[0]
	default:
		return ", " + strings.Join(paths[:len(paths)-1], ", ") + ", and " + paths[len(paths)-1]
	}
}

// isVSCodeExtensionInstalled reports whether the given extension ID (e.g.
// "shevaua.phpcs") is registered in any known VSCode variant's
// extensions.json manifest — the source of truth VSCode itself uses, as
// opposed to just a same-named folder existing on disk.
func isVSCodeExtensionInstalled(extensionID string) bool {
	return extensionInstalledInDirs(extensionID, vscodeExtensionsDirs())
}

// vscodeExtensionsDirs lists candidate extensions directories across VSCode
// variants (stable, Insiders, VSCodium, Code - OSS).
func vscodeExtensionsDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	names := []string{".vscode", ".vscode-insiders", ".vscodium", ".vscode-oss"}
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		dirs = append(dirs, filepath.Join(home, name, "extensions"))
	}
	return dirs
}

// vscodeExtensionManifestEntry is one entry of an extensions.json manifest.
type vscodeExtensionManifestEntry struct {
	Identifier struct {
		ID string `json:"id"`
	} `json:"identifier"`
}

// extensionInstalledInDirs reports whether extensionID is registered in the
// extensions.json manifest of any of dirs. extensions.json — not the presence
// of an "<extensionID>-<version>" folder — is what VSCode actually reads at
// startup to decide which extensions to load: an install that never reached
// a running window's live state (e.g. one run from an external process
// against an already-open window) can leave files on disk without ever being
// added to this manifest, which looks installed but isn't.
func extensionInstalledInDirs(extensionID string, dirs []string) bool {
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, "extensions.json"))
		if err != nil {
			continue
		}

		var entries []vscodeExtensionManifestEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			continue
		}

		for _, entry := range entries {
			if strings.EqualFold(entry.Identifier.ID, extensionID) {
				return true
			}
		}
	}
	return false
}

// ExtensionInstalledInDirsForTest exposes extensionInstalledInDirs to the tests package.
func ExtensionInstalledInDirsForTest(extensionID string, dirs []string) bool {
	return extensionInstalledInDirs(extensionID, dirs)
}

// vscodeGlobalSettingsPath returns the path to VSCode's user settings.json,
// preferring an existing install (Code, Code - Insiders, VSCodium, Code -
// OSS) and falling back to the standard Code location.
func vscodeGlobalSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}

	var base string
	switch runtime.GOOS {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
	default:
		base = filepath.Join(home, ".config")
	}

	candidates := []string{"Code", "Code - Insiders", "VSCodium", "Code - OSS"}
	for _, name := range candidates {
		path := filepath.Join(base, name, "User", "settings.json")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// None found yet (fresh install) — default to stable Code.
	return filepath.Join(base, candidates[0], "User", "settings.json"), nil
}

// normalizeIntelephensePHPVersion converts a "8.2"-style php_version into the
// "8.2.0"-style value intelephense.environment.phpVersion expects. Returns ""
// if no usable version is configured (e.g. php_version: none).
func normalizeIntelephensePHPVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "none" {
		return ""
	}
	if strings.Count(version, ".") == 1 {
		return version + ".0"
	}
	return version
}

// phpcsAvailable reports whether the project at root has phpcs installed via
// Composer.
func phpcsAvailable(root string) bool {
	_, err := os.Stat(filepath.Join(root, "vendor", "bin", "phpcs"))
	return err == nil
}

// phpstanAvailable reports whether the project at root has phpstan installed
// via Composer.
func phpstanAvailable(root string) bool {
	_, err := os.Stat(filepath.Join(root, "vendor", "bin", "phpstan"))
	return err == nil
}

// phpstanDefaultConfigFilenames are the config filenames PHPStan itself looks
// for by default (and what phpstan-vscode's own default phpcs.configFile
// search list matches), in priority order.
var phpstanDefaultConfigFilenames = []string{"phpstan.neon", "phpstan.neon.dist", "phpstan.dist.neon"}

// phpstanDefaultPaths mirrors the framework/paths convention `govard test
// phpstan` already uses when no paths are given on the command line, so this
// doesn't invent a new, inconsistent convention.
func phpstanDefaultPaths(framework string) []string {
	if framework == "magento2" {
		return []string{"app/code", "app/design"}
	}
	return []string{"app", "src"}
}

// hasPHPStanConfig reports whether the project at root already has one of
// PHPStan's own default config filenames.
func hasPHPStanConfig(root string) bool {
	for _, name := range phpstanDefaultConfigFilenames {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// phpstanDefaultOptions returns the --level/--autoload-file/paths CLI
// arguments to set as phpstan.options when the project has no phpstan.neon/
// .dist config of its own. This is set in .vscode/settings.json rather than
// written as a phpstan.neon at the project root, since that file is normally
// git-tracked and not ours to create.
func phpstanDefaultOptions(framework string) []string {
	options := []string{"--level=0", "--autoload-file=vendor/autoload.php"}
	return append(options, phpstanDefaultPaths(framework)...)
}

// HasPHPStanConfigForTest exposes hasPHPStanConfig to the tests package.
func HasPHPStanConfigForTest(root string) bool {
	return hasPHPStanConfig(root)
}

// PhpstanDefaultOptionsForTest exposes phpstanDefaultOptions to the tests package.
func PhpstanDefaultOptionsForTest(framework string) []string {
	return phpstanDefaultOptions(framework)
}

// phpunitAvailable reports whether the project at root has phpunit installed
// via Composer.
func phpunitAvailable(root string) bool {
	_, err := os.Stat(filepath.Join(root, "vendor", "bin", "phpunit"))
	return err == nil
}

// composerCodingStandardPackages maps known Composer packages that register a
// phpcs coding standard to the standard name to pass as --standard. Checked
// in order; the first match wins.
var composerCodingStandardPackages = []struct {
	Package  string
	Standard string
}{
	{Package: "magento/magento-coding-standard", Standard: "Magento2"},
	{Package: "wp-coding-standards/wpcs", Standard: "WordPress"},
	{Package: "drupal/coder", Standard: "Drupal"},
}

// detectPHPCSStandard picks a phpcs coding standard for the project at root
// by checking composer.json for a known coding-standard package, falling
// back to PSR12 (always available in squizlabs/php_codesniffer) if none of
// them are required.
func detectPHPCSStandard(root string) string {
	const fallback = "PSR12"

	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return fallback
	}

	var composer struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return fallback
	}

	for _, candidate := range composerCodingStandardPackages {
		if _, ok := composer.Require[candidate.Package]; ok {
			return candidate.Standard
		}
		if _, ok := composer.RequireDev[candidate.Package]; ok {
			return candidate.Standard
		}
	}
	return fallback
}

// DetectPHPCSStandardForTest exposes detectPHPCSStandard to the tests package.
func DetectPHPCSStandardForTest(root string) string {
	return detectPHPCSStandard(root)
}

// NormalizeIntelephensePHPVersionForTest exposes normalizeIntelephensePHPVersion to the tests package.
func NormalizeIntelephensePHPVersionForTest(version string) string {
	return normalizeIntelephensePHPVersion(version)
}

// MergeJSONObjectFileForTest exposes mergeJSONObjectFile to the tests package.
func MergeJSONObjectFileForTest(path string, set map[string]interface{}, unset []string) error {
	return mergeJSONObjectFile(path, set, unset)
}

// MergeLaunchConfigForTest exposes mergeLaunchConfig to the tests package.
func MergeLaunchConfigForTest(path, workdir string) error {
	return mergeLaunchConfig(path, workdir)
}

// ensureWrapperScript writes an executable one-line wrapper that delegates to
// `govard vscode <subcommand>`, used for editor settings that require a
// single executable path rather than a command array.
func ensureWrapperScript(path, subcommand string) error {
	body := fmt.Sprintf("#!/usr/bin/env bash\nexec govard vscode %s \"$@\"\n", subcommand)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// mergeJSONObjectFile reads path as a JSON object (treating a missing file as
// empty), applies each entry in set, removes each key in unset, and writes
// the result back with indentation. Keys are re-sorted alphabetically by
// encoding/json; unrelated existing keys are preserved.
func mergeJSONObjectFile(path string, set map[string]interface{}, unset []string) error {
	obj := map[string]interface{}{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("parse existing %s (comments/trailing commas aren't supported): %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	for _, key := range unset {
		delete(obj, key)
	}
	for key, value := range set {
		obj[key] = value
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	data, err := json.MarshalIndent(obj, "", "    ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// mergeLaunchConfig ensures launch.json at path contains a "Listen for
// Xdebug (Govard)" configuration with the given container workdir mapped to
// ${workspaceFolder}. Any other configurations are left untouched.
func mergeLaunchConfig(path, workdir string) error {
	launch := struct {
		Version        string                   `json:"version"`
		Configurations []map[string]interface{} `json:"configurations"`
	}{Version: "0.2.0"}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &launch); err != nil {
			return fmt.Errorf("parse existing %s (comments/trailing commas aren't supported): %w", path, err)
		}
		if launch.Version == "" {
			launch.Version = "0.2.0"
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	xdebugConfig := map[string]interface{}{
		"name":    xdebugLaunchConfigName,
		"type":    "php",
		"request": "launch",
		"port":    9003,
		"pathMappings": map[string]interface{}{
			workdir: "${workspaceFolder}",
		},
	}

	replaced := false
	for i, c := range launch.Configurations {
		if name, _ := c["name"].(string); name == xdebugLaunchConfigName {
			launch.Configurations[i] = xdebugConfig
			replaced = true
			break
		}
	}
	if !replaced {
		launch.Configurations = append(launch.Configurations, xdebugConfig)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	data, err := json.MarshalIndent(launch, "", "    ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
