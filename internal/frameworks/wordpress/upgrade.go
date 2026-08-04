package wordpress

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pterm/pterm"
	"govard/internal/conventions"
	"govard/internal/engine"
)

// Upgrade is wordpress's engine.UpgradeFunc, moved verbatim from
// engine.upgradeWordPress.
func Upgrade(ctx context.Context, config engine.Config, opts engine.UpgradeOptions) error {
	pterm.Info.Println("WordPress Upgrade Pipeline")
	containerName := fmt.Sprintf("%s%s", opts.ProjectName, conventions.PHPSuffix)

	if opts.TargetVersion == "" {
		return fmt.Errorf("target version is required. Example: govard upgrade --version=6.7")
	}

	if opts.DryRun {
		pterm.Info.Println("[DRY RUN] Would perform the following steps:")
		pterm.Info.Printf("  1. wp core update --version=%s\n", opts.TargetVersion)
		pterm.Info.Println("  2. wp core update-db")
		pterm.Info.Println("  3. wp cache flush")
		return nil
	}

	if !opts.NoInteraction {
		confirmed, _ := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(true).
			Show(fmt.Sprintf("Upgrade WordPress to %s?", opts.TargetVersion))
		if !confirmed {
			return fmt.Errorf("upgrade cancelled by user")
		}
	}

	// Step 1: WP core update
	pterm.Info.Println("Step 1/3: Updating WordPress core...")
	updateCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "wp", "core", "update", "--version="+opts.TargetVersion)
	updateCmd.Stdout = opts.Stdout
	updateCmd.Stderr = opts.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("wp core update failed: %w", err)
	}

	// Step 2: WP core update-db
	pterm.Info.Println("Step 2/3: Updating database...")
	dbCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "wp", "core", "update-db")
	dbCmd.Stdout = opts.Stdout
	dbCmd.Stderr = opts.Stderr
	if err := dbCmd.Run(); err != nil {
		pterm.Warning.Printf("wp core update-db failed: %v\n", err)
	}

	// Step 3: WP cache flush
	pterm.Info.Println("Step 3/3: Flushing cache...")
	flushCmd := exec.CommandContext(ctx, "docker", "exec", "-w", conventions.DefaultWorkDir, containerName, "wp", "cache", "flush")
	flushCmd.Stdout = opts.Stdout
	flushCmd.Stderr = opts.Stderr
	if err := flushCmd.Run(); err != nil {
		pterm.Warning.Printf("wp cache flush failed: %v\n", err)
	}

	pterm.Success.Printf("✅ WordPress upgrade to %s completed!\n", opts.TargetVersion)
	return nil
}
