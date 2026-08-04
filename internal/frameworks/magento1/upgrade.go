package magento1

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pterm/pterm"
	"govard/internal/conventions"
	"govard/internal/engine"
)

// Upgrade is magento1's engine.UpgradeFunc, moved verbatim from
// engine.upgradeMagento1.
func Upgrade(ctx context.Context, config engine.Config, opts engine.UpgradeOptions) error {
	pterm.Info.Println("Magento 1 Upgrade Pipeline")
	containerName := fmt.Sprintf("%s%s", opts.ProjectName, conventions.PHPSuffix)

	if opts.DryRun {
		pterm.Info.Println("[DRY RUN] Would perform the following steps:")
		pterm.Info.Println("  1. composer install (if composer.json exists)")
		pterm.Info.Println("  2. Clear compiler cache (shell/compiler.php clear)")
		pterm.Info.Println("  3. Flush var/cache, var/session, etc.")
		pterm.Info.Println("  4. Running database upgrades via n98-magerun sys:setup:run (if available)")
		return nil
	}

	if !opts.NoInteraction {
		confirmed, _ := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(true).
			Show("Proceed with Magento 1 upgrade steps (Composer install, Cache flush, DB setup)?")
		if !confirmed {
			return fmt.Errorf("upgrade cancelled by user")
		}
	}

	// Step 1: Composer install
	pterm.Info.Println("Step 1/4: Installing composer dependencies...")
	checkComposer := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "test", "-f", "composer.json")
	if err := checkComposer.Run(); err == nil {
		composerCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "install")
		composerCmd.Stdout = opts.Stdout
		composerCmd.Stderr = opts.Stderr
		if err := composerCmd.Run(); err != nil {
			pterm.Warning.Printf("Composer install failed: %v\n", err)
		}
	}

	// Step 2: Clear compiler cache
	pterm.Info.Println("Step 2/4: Clearing compiler cache...")
	checkCompiler := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "test", "-f", "shell/compiler.php")
	if err := checkCompiler.Run(); err == nil {
		compilerCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "php", "shell/compiler.php", "clear")
		if err := compilerCmd.Run(); err != nil {
			pterm.Warning.Printf("Compiler clear failed: %v\n", err)
		}
	}

	// Step 3: Flush cache
	pterm.Info.Println("Step 3/4: Flushing cache...")
	flushCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "bash", "-c", "rm -rf var/cache/* var/session/* var/full_page_cache/* media/css/* media/js/*")
	if err := flushCmd.Run(); err != nil {
		pterm.Warning.Printf("Cache flush failed: %v\n", err)
	}

	// Step 4: Database upgrades
	pterm.Info.Println("Step 4/4: Running database upgrades...")
	checkMagerun := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "command", "-v", "n98-magerun")
	if err := checkMagerun.Run(); err == nil {
		magerunCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "n98-magerun", "sys:setup:run")
		magerunCmd.Stdout = opts.Stdout
		magerunCmd.Stderr = opts.Stderr
		if err := magerunCmd.Run(); err != nil {
			pterm.Warning.Printf("n98-magerun setup:run failed: %v\n", err)
		}
	}

	pterm.Success.Println("✅ Magento 1 upgrade completed!")
	return nil
}
