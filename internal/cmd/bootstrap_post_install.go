package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runBootstrapHyvaInstall(cmd *cobra.Command, opts bootstrapRuntimeOptions) error {
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

func runBootstrapMagentoSetupInstall(cmd *cobra.Command, config engine.Config, opts bootstrapRuntimeOptions) error {
	emailDomain := config.Domain
	if emailDomain == "" {
		// Used in older templates, keeping logic if extending later
		_ = "local.test"
	}

	setupArgs := []string{
		"setup:install",
		"--backend-frontname=admin",
		"--db-host=db",
		"--db-name=magento",
		"--db-user=magento",
		"--db-password=magento",
		"--db-prefix=" + strings.TrimSpace(os.Getenv("DB_PREFIX")),
		"--search-engine=opensearch",
		"--opensearch-host=elasticsearch",
		"--opensearch-port=9200",
		"--opensearch-index-prefix=magento2",
		"--opensearch-enable-auth=0",
		"--opensearch-timeout=15",
		"--admin-user=admin",
		"--admin-password=Admin123$",
		"--admin-firstname=Admin",
		"--admin-lastname=User",
		"--admin-email=admin@" + emailDomain,
	}

	if opts.MetaVersion != "" {
		if comparison, comparable := compareNumericDotVersions(opts.MetaVersion, "2.4.8"); comparable && comparison < 0 {
			setupArgs = []string{
				"setup:install",
				"--backend-frontname=admin",
				"--db-host=db",
				"--db-name=magento",
				"--db-user=magento",
				"--db-password=magento",
				"--db-prefix=" + strings.TrimSpace(os.Getenv("DB_PREFIX")),
				"--search-engine=elasticsearch7",
				"--elasticsearch-host=elasticsearch",
				"--elasticsearch-port=9200",
				"--elasticsearch-index-prefix=magento2",
				"--elasticsearch-enable-auth=0",
				"--elasticsearch-timeout=15",
				"--admin-user=admin",
				"--admin-password=Admin123$",
				"--admin-firstname=Admin",
				"--admin-lastname=User",
				"--admin-email=admin@" + emailDomain,
			}
		}
	}

	containerName := fmt.Sprintf("%s-php-1", config.ProjectName)
	if engine.IsContainerRunning(context.Background(), containerName) {
		esFixCmd := []string{
			"exec", "-i", containerName, "sh", "-c",
			"curl -s -X PUT 'http://elasticsearch:9200/_all/_settings' -H 'Content-Type: application/json' -d'{\"index.blocks.read_only_allow_delete\": null}' > /dev/null 2>&1 || true",
		}
		if err := runGovardSubcommand(cmd, append([]string{"custom"}, esFixCmd...)...); err != nil {
			pterm.Warning.Printf("Failed to apply Elasticsearch block fix: %v\n", err)
		}
	}

	if err := runGovardSubcommand(cmd, govardMagentoSubcommandArgs(setupArgs...)...); err != nil {
		return fmt.Errorf("magento setup:install failed: %w", err)
	}
	return nil
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

	containerName := fmt.Sprintf("%s-php-1", projectName)
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

	containerName := fmt.Sprintf("%s-php-1", projectName)
	if !engine.IsContainerRunning(context.Background(), containerName) {
		pterm.Warning.Printf("Skipping admin user creation: container %s is not running\n", containerName)
		return
	}

	emailDomain := config.Domain
	if emailDomain == "" {
		// Used in older templates, keeping logic if extending later
		_ = "local.test"
	}

	pterm.Info.Println("Creating Magento admin user...")
	err := runGovardSubcommand(
		cmd,
		govardMagentoSubcommandArgs(
			"admin:user:create",
			"--admin-user="+engine.DefaultMagentoAdminUser,
			"--admin-password="+engine.DefaultMagentoAdminPassword,
			"--admin-firstname=Govard",
			"--admin-lastname=Admin",
			"--admin-email="+engine.DefaultMagentoAdminEmail,
		)...,
	)
	if err != nil {
		pterm.Warning.Printf("Admin user creation skipped: %v\n", err)
	}
}

func runBootstrapFixDeps(cmd *cobra.Command, opts bootstrapRuntimeOptions) {
	args := []string{"custom", "fix-deps"}
	if opts.MetaVersion != "" {
		args = append(args, "--", "--framework-version="+opts.MetaVersion)
	}
	if err := runGovardSubcommand(cmd, args...); err != nil {
		pterm.Warning.Printf("fix-deps step skipped: %v\n", err)
	}
}

func maybeAutoDetectBootstrapVersion(config engine.Config, opts *bootstrapRuntimeOptions) {
	if opts == nil {
		return
	}
	if !opts.Clone || !opts.FixDeps || strings.TrimSpace(opts.MetaVersion) != "" {
		return
	}

	remoteCfg, ok := config.Remotes[opts.Source]
	if !ok {
		return
	}

	detectedVersion, err := remote.DetectMagento2Version(opts.Source, remoteCfg)
	if err != nil {
		pterm.Warning.Printf("Could not auto-detect Magento version from remote '%s' for fix-deps (%v).\n", opts.Source, err)
		return
	}
	opts.MetaVersion = detectedVersion
	pterm.Info.Printf("Detected remote Magento version %s from '%s' for fix-deps.\n", detectedVersion, opts.Source)
}

// RunBootstrapHyvaInstallForTest exposes runBootstrapHyvaInstall for tests in /tests.
func RunBootstrapHyvaInstallForTest(cmd *cobra.Command, hyvaToken string) error {
	return runBootstrapHyvaInstall(cmd, bootstrapRuntimeOptions{
		HyvaToken: strings.TrimSpace(hyvaToken),
	})
}

// RunBootstrapMagentoSetupInstallForTest exposes runBootstrapMagentoSetupInstall for tests in /tests.
func RunBootstrapMagentoSetupInstallForTest(cmd *cobra.Command, config engine.Config, frameworkVersion string) error {
	return runBootstrapMagentoSetupInstall(cmd, config, bootstrapRuntimeOptions{
		MetaVersion: strings.TrimSpace(frameworkVersion),
	})
}

// RunBootstrapSampleDataForTest exposes runBootstrapSampleData for tests in /tests.
func RunBootstrapSampleDataForTest(cmd *cobra.Command) error {
	return runBootstrapSampleData(cmd)
}

// RunBootstrapFixDepsForTest exposes runBootstrapFixDeps for tests in /tests.
func RunBootstrapFixDepsForTest(cmd *cobra.Command, frameworkVersion string) {
	runBootstrapFixDeps(cmd, bootstrapRuntimeOptions{
		MetaVersion: strings.TrimSpace(frameworkVersion),
	})
}
