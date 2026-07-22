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

func runBootstrapPostInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	adminEmail := conventions.AdminEmailForDomain(config.Domain)

	dbName := conventions.DefaultMagentoDBName
	dbUser := conventions.DefaultMagentoDBUser
	dbPass := conventions.DefaultMagentoDBPass
	if config.Framework == "mageos" {
		dbName = conventions.DefaultMageOSDBName
		dbUser = conventions.DefaultMageOSDBUser
		dbPass = conventions.DefaultMageOSDBPass
	}

	tablePrefix, err := resolveBootstrapMagentoTablePrefix(config)
	if err != nil {
		return err
	}
	setupArgs := []string{
		"setup:install",
		"--backend-frontname=" + conventions.DefaultAdminPath,
		"--db-host=" + conventions.DefaultMagentoDBHost,
		"--db-name=" + dbName,
		"--db-user=" + dbUser,
		"--db-password=" + dbPass,
		"--db-prefix=" + tablePrefix,
		"--search-engine=opensearch",
		"--opensearch-host=elasticsearch",
		"--opensearch-port=9200",
		"--opensearch-index-prefix=magento2",
		"--opensearch-enable-auth=0",
		"--opensearch-timeout=15",
		"--admin-user=" + conventions.DefaultAdminUser,
		"--admin-password=" + conventions.DefaultAdminPassword,
		"--admin-firstname=Admin",
		"--admin-lastname=User",
		"--admin-email=" + adminEmail,
	}

	if strings.EqualFold(config.Framework, conventions.FrameworkMagento2) && opts.MetaVersion != "" {
		if comparison, comparable := compareNumericDotVersions(opts.MetaVersion, "2.4.8"); comparable && comparison < 0 {
			setupArgs = []string{
				"setup:install",
				"--backend-frontname=" + conventions.DefaultAdminPath,
				"--db-host=" + conventions.DefaultMagentoDBHost,
				"--db-name=" + dbName,
				"--db-user=" + dbUser,
				"--db-password=" + dbPass,
				"--db-prefix=" + tablePrefix,
				"--search-engine=elasticsearch7",
				"--elasticsearch-host=elasticsearch",
				"--elasticsearch-port=9200",
				"--elasticsearch-index-prefix=magento2",
				"--elasticsearch-enable-auth=0",
				"--elasticsearch-timeout=15",
				"--admin-user=" + conventions.DefaultAdminUser,
				"--admin-password=" + conventions.DefaultAdminPassword,
				"--admin-firstname=Admin",
				"--admin-lastname=User",
				"--admin-email=" + adminEmail,
			}
		}
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	if engine.IsContainerRunning(context.Background(), containerName) {
		esFixCmd := []string{
			"exec", "-T", "php", "sh", "-c",
			"curl -s -X PUT 'http://elasticsearch:9200/_all/_settings' -H 'Content-Type: application/json' -d'{\"index.blocks.read_only_allow_delete\": null}' > /dev/null 2>&1 || true",
		}
		if err := runGovardSubcommand(cmd, append([]string{"env"}, esFixCmd...)...); err != nil {
			pterm.Warning.Printf("Failed to apply Elasticsearch block fix: %v\n", err)
		}
	}

	if err := runGovardSubcommand(cmd, govardMagentoSubcommandArgs(setupArgs...)...); err != nil {
		return fmt.Errorf("magento setup:install failed: %w", err)
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

// RunBootstrapMagentoSetupInstallForTest exposes runBootstrapPostInstall for tests in /tests.
func RunBootstrapMagentoSetupInstallForTest(cmd *cobra.Command, config engine.Config, source, version string) error {
	return runBootstrapPostInstall(cmd, config, BootstrapRuntimeOptions{
		Source:          source,
		MetaVersion:     version,
		ComposerInstall: true,
	})
}

// RunBootstrapSampleDataForTest exposes runBootstrapSampleData for tests in /tests.
func RunBootstrapSampleDataForTest(cmd *cobra.Command) error {
	return runBootstrapSampleData(cmd)
}
