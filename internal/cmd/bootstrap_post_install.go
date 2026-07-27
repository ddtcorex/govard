package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runBootstrapHyvaInstall(cmd *cobra.Command, opts BootstrapRuntimeOptions) error {
	if err := runGovardSubcommand(
		cmd,
		govardComposerSubcommandArgs(
			"config",
			"http-basic.hyva-themes.repo.packagist.com",
			"token",
			opts.HyvaToken,
		)...,
	); err != nil {
		return fmt.Errorf("failed to set Hyva token: %w", err)
	}
	if err := runGovardSubcommand(
		cmd,
		govardComposerSubcommandArgs(
			"config",
			"repositories.hyva-themes",
			"composer",
			"https://hyva-themes.repo.packagist.com/app-hyva-test-dv1dgx/",
		)...,
	); err != nil {
		return fmt.Errorf("failed to add Hyva repository: %w", err)
	}
	if err := runGovardSubcommand(cmd, govardComposerSubcommandArgs("require", "-n", "hyva-themes/magento2-default-theme")...); err != nil {
		return fmt.Errorf("failed to install Hyva package: %w", err)
	}
	return nil
}

func resolveBootstrapMagentoTablePrefix(config engine.Config) (string, error) {
	if prefix := engine.NormalizeTablePrefix(config.TablePrefix); prefix != "" {
		if !engine.ValidateTablePrefix(prefix) {
			return "", fmt.Errorf("invalid table_prefix %q (allowed: letters, numbers, and underscore)", prefix)
		}
		return prefix, nil
	}
	prefix := engine.NormalizeTablePrefix(os.Getenv("TABLE_PREFIX"))
	if !engine.ValidateTablePrefix(prefix) {
		return "", fmt.Errorf("invalid TABLE_PREFIX %q (allowed: letters, numbers, and underscore)", prefix)
	}
	return prefix, nil
}

func runBootstrapSampleData(cmd *cobra.Command) error {
	commands := [][]string{
		{"sample:deploy"},
		{"setup:upgrade"},
		{"indexer:reindex"},
		{"cache:flush"},
	}
	for _, args := range commands {
		if err := runGovardSubcommand(cmd, govardMagentoSubcommandArgs(args...)...); err != nil {
			return fmt.Errorf("sample data step failed (%s): %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

func runBootstrapMagentoReindex(cmd *cobra.Command) error {
	pterm.Info.Println("Reindexing Magento data...")
	cwd, _ := os.Getwd()
	projectName := filepath.Base(cwd)
	if cfg, _, err := engine.LoadConfigFromDir(cwd, false); err == nil && strings.TrimSpace(cfg.ProjectName) != "" {
		projectName = cfg.ProjectName
	}

	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	if !engine.IsContainerRunning(context.Background(), containerName) {
		pterm.Warning.Printf("Skipping reindex: container %s is not running\n", containerName)
		return nil
	}

	if err := runGovardSubcommand(cmd, govardMagentoSubcommandArgs("indexer:reindex")...); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}
	return nil
}

func runBootstrapAdminCreate(cmd *cobra.Command, config engine.Config) {
	cwd, _ := os.Getwd()
	projectName := filepath.Base(cwd)
	if strings.TrimSpace(config.ProjectName) != "" {
		projectName = config.ProjectName
	}

	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	if !engine.IsContainerRunning(context.Background(), containerName) {
		pterm.Warning.Printf("Skipping admin user creation: container %s is not running\n", containerName)
		return
	}

	pterm.Info.Println("Creating Magento admin user...")
	err := runGovardSubcommandSilent(
		cmd,
		govardMagentoSubcommandArgs(
			"admin:user:create",
			"--admin-user="+conventions.DefaultAdminUser,
			"--admin-password="+conventions.DefaultAdminPassword,
			"--admin-firstname=Govard",
			"--admin-lastname=Admin",
			"--admin-email="+conventions.AdminEmailForDomain(config.Domain),
		)...,
	)
	if err != nil {
		pterm.Warning.Printf("Admin user creation skipped: %v\n", err)
	}
}

// RunBootstrapHyvaInstallForTest exposes runBootstrapHyvaInstall for tests in /tests.
func RunBootstrapHyvaInstallForTest(cmd *cobra.Command, hyvaToken string) error {
	return runBootstrapHyvaInstall(cmd, BootstrapRuntimeOptions{
		HyvaToken: strings.TrimSpace(hyvaToken),
	})
}

// RunBootstrapSampleDataForTest exposes runBootstrapSampleData for tests in /tests.
func RunBootstrapSampleDataForTest(cmd *cobra.Command) error {
	return runBootstrapSampleData(cmd)
}
