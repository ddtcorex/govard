package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"strings"
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

// RunBootstrapHyvaInstallForTest exposes runBootstrapHyvaInstall for tests in /tests.
func RunBootstrapHyvaInstallForTest(cmd *cobra.Command, hyvaToken string) error {
	return runBootstrapHyvaInstall(cmd, BootstrapRuntimeOptions{
		HyvaToken: strings.TrimSpace(hyvaToken),
	})
}
