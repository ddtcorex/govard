package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

// magentoUpgradeVariant parameterizes the shared Magento-2-family upgrade
// pipeline for a specific distribution (Magento 2 Open Source/Commerce, or
// Mage-OS).
type magentoUpgradeVariant struct {
	DisplayName   string
	Metapackage   string
	RepositoryURL string
	PackagePrefix string
}

var magento2UpgradeVariant = magentoUpgradeVariant{
	DisplayName:   "Magento 2",
	Metapackage:   "magento/project-community-edition",
	RepositoryURL: "https://repo.magento.com/",
	PackagePrefix: "magento/",
}

var mageOSUpgradeVariant = magentoUpgradeVariant{
	DisplayName:   "Mage-OS",
	Metapackage:   "mage-os/project-community-edition",
	RepositoryURL: "https://repo.mage-os.org/",
	PackagePrefix: "mage-os/",
}

func upgradeMagento2(ctx context.Context, config Config, opts UpgradeOptions, variant magentoUpgradeVariant) error {
	containerName := fmt.Sprintf("%s%s", opts.ProjectName, conventions.PHPSuffix)

	if opts.TargetVersion == "" {
		return fmt.Errorf("target version is required. Example: govard upgrade --version=2.4.8-p4")
	}

	pterm.Info.Printf("Target version: %s\n", opts.TargetVersion)

	currentVersion, _ := getMagentoCurrentVersion(containerName)
	if currentVersion == "" {
		currentVersion = "unknown"
	}
	pterm.Info.Printf("Current version: %s\n", currentVersion)

	if opts.DryRun {
		pterm.Info.Println("[DRY RUN] Would perform the following steps:")
		pterm.Info.Println("  1. Update .govard.yml configuration for the target framework version")
		pterm.Info.Println("  2. Restart environment (govard env down && govard env up)")
		pterm.Info.Printf("  3. Create temporary %s project %s to fetch composer.json\n", variant.DisplayName, opts.TargetVersion)
		pterm.Info.Println("  4. Merge composer.json preserving 3rd-party dependencies")
		pterm.Info.Printf("  5. Run composer update %s* phpunit/* --with-all-dependencies\n", variant.PackagePrefix)
		pterm.Info.Println("  6. bin/magento setup:upgrade, setup:di:compile, cache:flush")
		return nil
	}

	if !opts.NoInteraction {
		pterm.Warning.Println("This will update framework profile dependencies, restart the environment, and modify composer.json.")
		confirm, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).Show(fmt.Sprintf("Proceed with upgrade to %s?", opts.TargetVersion))
		if !confirm {
			pterm.Info.Println("Upgrade cancelled.")
			return nil
		}
	}

	// Step 1: Env update
	if !opts.NoEnvUpdate {
		pterm.Info.Println("Step 1/6: Applying runtime profile for target version...")
		targetProfile, err := ResolveRuntimeProfile(config.Framework, opts.TargetVersion)
		if err != nil {
			pterm.Warning.Printf("Could not resolve specific profile for %s (continuing): %v\n", opts.TargetVersion, err)
		} else {
			ApplyRuntimeProfileToConfig(&config, targetProfile.Profile)
			NormalizeConfig(&config, opts.ProjectDir)
			// Ensure it writes to file
			cleanConfig := PrepareConfigForWrite(config)
			yamlOut, err := yaml.Marshal(&cleanConfig)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}
			if err := os.WriteFile(filepath.Join(opts.ProjectDir, ".govard.yml"), yamlOut, conventions.DefaultFilePerm); err != nil {
				return fmt.Errorf("failed to write .govard.yml: %w", err)
			}

			if err := RenderBlueprint(opts.ProjectDir, config); err != nil {
				return fmt.Errorf("failed to render environment: %w", err)
			}
		}

		pterm.Info.Println("Step 2/6: Restarting environment (PHP, DB, Cache, Search)...")
		composePath := ComposeFilePathWithProfile(opts.ProjectDir, opts.ProjectName, config.Profile)
		if err := RunCompose(ctx, ComposeOptions{
			ProjectDir:  opts.ProjectDir,
			ProjectName: opts.ProjectName,
			ComposeFile: composePath,
			Args:        []string{"down"},
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		}); err != nil {
			pterm.Warning.Printf("Failed to stop environment: %v\n", err)
		}
		if err := RunCompose(ctx, ComposeOptions{
			ProjectDir:  opts.ProjectDir,
			ProjectName: opts.ProjectName,
			ComposeFile: composePath,
			Args:        []string{"up", "-d"},
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		}); err != nil {
			return fmt.Errorf("failed to start environment: %w", err)
		}

		pterm.Info.Println("Waiting for database to be ready...")
		checkDatabaseReady(ctx, config, containerName)
	}

	if err := FixComposerCompatibility(config); err != nil {
		return fmt.Errorf("failed to fix composer compatibility: %w", err)
	}

	pterm.Info.Println("Step 3/6: Fetching and merging composer.json...")

	if err := updateMagentoComposerJson(opts, containerName, variant); err != nil {
		return fmt.Errorf("failed to update composer.json: %w", err)
	}

	pterm.Info.Println("Step 4/6: Running composer update...")
	// Relax some packages
	relaxed := relaxPackages(containerName)

	// Composer update
	updatePkgs := []string{variant.PackagePrefix + "*", "phpunit/*"}
	for _, r := range relaxed {
		pkgName := strings.Split(r, ":")[0]
		// Avoid duplicates and wildcards already covered
		if pkgName == "phpunit/phpunit" {
			continue
		}
		updatePkgs = append(updatePkgs, pkgName)
	}

	cmdArgs := append([]string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "update"}, updatePkgs...)
	cmdArgs = append(cmdArgs, "--with-all-dependencies", "--ignore-platform-reqs", "--no-install")
	updateCmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	updateCmd.Stdout = opts.Stdout
	updateCmd.Stderr = opts.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("composer update failed: %w", err)
	}

	pterm.Info.Println("Synchronizing dependencies safely (two-phase install)...")
	if err := runMagentoComposerInstall(opts.ProjectName, config, opts.Stdout, opts.Stderr); err != nil {
		return fmt.Errorf("safe composer installation failed: %w", err)
	}

	pterm.Info.Println("Cleaning up generated code and cache before database upgrade...")
	if err := wipeMagentoGeneratedCaches(opts.ProjectName, config); err != nil {
		pterm.Warning.Printf("Failed to wipe generated directories (continuing): %v\n", err)
	}

	pterm.Info.Println("Step 5/6: Running setup:upgrade...")
	if !opts.NoDBUpgrade {
		suArgs := []string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinMagento, "setup:upgrade", "--no-interaction"}
		su := exec.CommandContext(ctx, "docker", suArgs...)
		su.Stdout = opts.Stdout
		su.Stderr = opts.Stderr
		if err := su.Run(); err != nil {
			return fmt.Errorf("setup:upgrade failed: %w", err)
		}
	} else {
		pterm.Info.Println("Skipped (--no-db-upgrade)")
	}

	pterm.Info.Println("Step 6/6: Compiling and flushing cache...")
	diArgs := []string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinMagento, "setup:di:compile"}
	diCmd := exec.CommandContext(ctx, "docker", diArgs...)
	if err := diCmd.Run(); err != nil {
		pterm.Warning.Printf("setup:di:compile failed: %v\n", err)
	}

	cacheArgs := []string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinMagento, "cache:flush"}
	cacheCmd := exec.CommandContext(ctx, "docker", cacheArgs...)
	if err := cacheCmd.Run(); err != nil {
		pterm.Warning.Printf("cache:flush failed: %v\n", err)
	}

	// Clean backup
	_ = os.Remove(filepath.Join(opts.ProjectDir, "composer.json.bak"))

	pterm.Success.Printf("✅ %s upgrade to %s completed!\n", variant.DisplayName, opts.TargetVersion)
	return nil
}

func getMagentoCurrentVersion(containerName string) (string, error) {
	cmdArgs := []string{"exec", "-w", conventions.DefaultWorkDir, containerName, "php", conventions.BinMagento, "--version"}
	out, err := exec.Command("docker", cmdArgs...).CombinedOutput()
	if err == nil {
		re := regexp.MustCompile(`\d+\.\d+\.\d+(-p\d+)?`)
		match := re.FindString(string(out))
		if match != "" {
			return match, nil
		}
	}
	return "", fmt.Errorf("could not detect")
}

func updateMagentoComposerJson(opts UpgradeOptions, containerName string, variant magentoUpgradeVariant) error {
	composerPath := filepath.Join(opts.ProjectDir, "composer.json")
	backupPath := filepath.Join(opts.ProjectDir, "composer.json.bak")

	// Backup
	b, err := os.ReadFile(composerPath)
	if err == nil {
		if err := os.WriteFile(backupPath, b, conventions.DefaultFilePerm); err != nil {
			pterm.Warning.Printf("Failed to create backup: %v\n", err)
		}
	}

	projCmd := exec.Command("docker", "exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "create-project", "--no-install", "--ignore-platform-reqs", "--repository="+variant.RepositoryURL, variant.Metapackage, "temp_upgrade_source", opts.TargetVersion)
	projCmd.Stdout = opts.Stdout
	projCmd.Stderr = opts.Stderr
	if err := projCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch composer.json for %s", opts.TargetVersion)
	}

	newComposerBytes, err := exec.Command("docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "cat", "temp_upgrade_source/composer.json").Output()
	_ = exec.Command("docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "rm", "-rf", "temp_upgrade_source").Run()

	if err != nil {
		return fmt.Errorf("failed to read fetched composer.json")
	}

	var currentMap, newMap map[string]interface{}
	if err := json.Unmarshal(b, &currentMap); err != nil {
		return err
	}
	if err := json.Unmarshal(newComposerBytes, &newMap); err != nil {
		return err
	}

	// Merge require, require-dev, conflict, extra
	mergeComposerMapKeys(currentMap, newMap, "require", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "require-dev", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "conflict", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "autoload", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "minimum-stability", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "prefer-stable", variant.PackagePrefix)
	mergeComposerMapKeys(currentMap, newMap, "extra", variant.PackagePrefix)

	mergedBytes, err := json.MarshalIndent(currentMap, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(composerPath, mergedBytes, conventions.DefaultFilePerm)
}

func mergeComposerMapKeys(current map[string]interface{}, target map[string]interface{}, key string, packagePrefix string) {
	if _, ok := target[key]; !ok {
		return
	}
	targetVal := target[key]

	// If it's not a map but a direct value (like string for minimum-stability)
	targetMap, isMap := targetVal.(map[string]interface{})
	if !isMap {
		current[key] = targetVal
		return
	}

	if _, currOk := current[key]; !currOk {
		current[key] = map[string]interface{}{}
	}

	currentMap, currIsMap := current[key].(map[string]interface{})
	if !currIsMap {
		current[key] = targetVal
		return
	}

	// For requirement sections, remove old <packagePrefix>* packages that are no longer in the target version
	if key == "require" || key == "require-dev" {
		for k := range currentMap {
			if strings.HasPrefix(k, packagePrefix) {
				if _, ok := targetMap[k]; !ok {
					delete(currentMap, k)
				}
			}
		}
	}

	for k, v := range targetMap {
		currentMap[k] = v
	}
}

func MergeComposerMapKeysForTest(current map[string]interface{}, target map[string]interface{}, key string) {
	mergeComposerMapKeys(current, target, key, "magento/")
}

// MergeComposerMapKeysWithPrefixForTest exposes mergeComposerMapKeys with an
// explicit stale-package prefix, for tests exercising non-magento/ prefixes.
func MergeComposerMapKeysWithPrefixForTest(current map[string]interface{}, target map[string]interface{}, key string, packagePrefix string) {
	mergeComposerMapKeys(current, target, key, packagePrefix)
}

func relaxPackages(containerName string) []string {
	// Check existing in composer.json
	cmdGet := exec.Command("docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "cat", "composer.json")
	out, err := cmdGet.Output()
	if err != nil {
		return nil
	}
	return RelaxPackagesFromContentForTest(string(out), containerName)
}

// RelaxPackagesFromContentForTest identifies packages to relax from composer.json content.
func RelaxPackagesFromContentForTest(content string, containerName string) []string {
	relax := []string{
		"phpunit/phpunit:*",
		"pdepend/pdepend:*",
		"phpmd/phpmd:*",
		"friendsofphp/php-cs-fixer:*",
		"magento/magento-coding-standard:*",
		"magento/magento-allure-phpunit:*",
		"magento/magento2-functional-testing-framework:*",
		"phpstan/phpstan:*",
		"symfony/finder:*",
		"symfony/process:*",
		"symfony/console:*",
		"symfony/yaml:*",
		"symfony/var-dumper:*",
		"symfony/event-dispatcher:*",
		"allure-framework/allure-phpunit:*",
		"sebastian/phpcpd:*",
		"sebastian/comparator:*",
		"sebastian/diff:*",
		"sebastian/exporter:*",
		"sebastian/recursion-context:*",
		"sebastian/code-unit:*",
		"sebastian/cli-parser:*",
		"sebastian/code-unit-reverse-lookup:*",
		"sebastian/complexity:*",
		"sebastian/environment:*",
		"sebastian/global-state:*",
		"sebastian/lines-of-code:*",
		"sebastian/object-enumerator:*",
		"sebastian/object-reflector:*",
		"sebastian/type:*",
		"sebastian/version:*",
		"laminas/laminas-dom:*",
		"laminas/laminas-escaper:*",
		"laminas/laminas-stdlib:*",
	}

	var toRelax, toRelaxDev []string
	for _, pkgStr := range relax {
		pkgName := strings.Split(pkgStr, ":")[0]
		// Find which section it belongs to
		reReq := regexp.MustCompile(`"require"\s*:\s*\{[^}]*"` + regexp.QuoteMeta(pkgName) + `"`)
		reDev := regexp.MustCompile(`"require-dev"\s*:\s*\{[^}]*"` + regexp.QuoteMeta(pkgName) + `"`)

		if reDev.MatchString(content) {
			toRelaxDev = append(toRelaxDev, pkgStr)
		} else if reReq.MatchString(content) {
			toRelax = append(toRelax, pkgStr)
		}
	}

	if containerName != "" {
		if len(toRelax) > 0 {
			args := append([]string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "require"}, toRelax...)
			args = append(args, "--no-update")
			if err := exec.Command("docker", args...).Run(); err != nil {
				pterm.Warning.Printf("Failed to relax packages: %v\n", err)
			}
		}
		if len(toRelaxDev) > 0 {
			args := append([]string{"exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "require", "--dev"}, toRelaxDev...)
			args = append(args, "--no-update")
			if err := exec.Command("docker", args...).Run(); err != nil {
				pterm.Warning.Printf("Failed to relax dev packages: %v\n", err)
			}
		}
	}

	return append(toRelax, toRelaxDev...)
}

func checkDatabaseReady(ctx context.Context, config Config, containerName string) {
	for i := 0; i < 30; i++ {
		cmdArgs := []string{"exec", "-w", conventions.DefaultWorkDir, containerName, "php", "-r", "$m=new mysqli('db', 'magento', 'magento'); if($m->connect_error) exit(1); exit(0);"}
		out := exec.CommandContext(ctx, "docker", cmdArgs...)
		if err := out.Run(); err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	pterm.Warning.Println("Database may not be ready, continuing anyway...")
}
