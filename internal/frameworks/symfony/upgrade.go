package symfony

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pterm/pterm"
	"govard/internal/conventions"
	"govard/internal/engine"
)

// Upgrade is symfony's engine.UpgradeFunc, moved verbatim from
// engine.upgradeSymfony.
func Upgrade(ctx context.Context, config engine.Config, opts engine.UpgradeOptions) error {
	pterm.Info.Println("Symfony Upgrade Pipeline")
	containerName := fmt.Sprintf("%s%s", opts.ProjectName, conventions.PHPSuffix)

	if opts.TargetVersion == "" {
		return fmt.Errorf("target version is required. Example: govard upgrade --version=7")
	}

	if opts.DryRun {
		pterm.Info.Println("[DRY RUN] Would perform the following steps:")
		pterm.Info.Printf("  1. composer require symfony/framework-bundle:^%s --no-update\n", opts.TargetVersion)
		pterm.Info.Println("  2. composer update")
		pterm.Info.Println("  3. php bin/console doctrine:migrations:migrate --no-interaction")
		pterm.Info.Println("  4. php bin/console cache:clear")
		return nil
	}

	if !opts.NoInteraction {
		confirmed, _ := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(true).
			Show(fmt.Sprintf("Upgrade Symfony to ^%s?", opts.TargetVersion))
		if !confirmed {
			return fmt.Errorf("upgrade cancelled by user")
		}
	}

	// Step 1: Composer require
	pterm.Info.Println("Step 1/4: Updating composer.json...")
	requireCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "require", fmt.Sprintf("symfony/framework-bundle:^%s", opts.TargetVersion), "--no-update")
	requireCmd.Stdout = opts.Stdout
	requireCmd.Stderr = opts.Stderr
	if err := requireCmd.Run(); err != nil {
		return fmt.Errorf("failed to update composer.json: %w", err)
	}

	// Step 2: Composer update
	pterm.Info.Println("Step 2/4: Running composer update...")
	updateCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, conventions.BinComposer, "update")
	updateCmd.Stdout = opts.Stdout
	updateCmd.Stderr = opts.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("composer update failed: %w", err)
	}

	// Step 3: Migration
	pterm.Info.Println("Step 3/4: Running migrations...")
	migrateCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "php", conventions.BinSymfonyConsole, "doctrine:migrations:migrate", "--no-interaction")
	migrateCmd.Stdout = opts.Stdout
	migrateCmd.Stderr = opts.Stderr
	if err := migrateCmd.Run(); err != nil {
		pterm.Warning.Printf("Migrations failed: %v\n", err)
	}

	// Step 4: Cache clear
	pterm.Info.Println("Step 4/4: Clearing cache...")
	cacheCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "php", conventions.BinSymfonyConsole, "cache:clear")
	cacheCmd.Stdout = opts.Stdout
	cacheCmd.Stderr = opts.Stderr
	if err := cacheCmd.Run(); err != nil {
		pterm.Warning.Printf("Cache clear failed: %v\n", err)
	}

	pterm.Success.Printf("✅ Symfony upgrade to ^%s completed!\n", opts.TargetVersion)
	return nil
}
