package cmd

import (
	"fmt"
	"os"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runBootstrapFrameworkFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()

	if config.Framework == "mageos" && opts.MetaPackage == defaultBootstrapMetaPackage {
		opts.MetaPackage = "mage-os/project-community-edition"
	}

	switch config.Framework {
	case "magento2":
		return runBootstrapMagentoFreshInstall(cmd, config, opts)
	case "mageos":
		return runBootstrapMageOSFreshInstall(cmd, config, opts)
	case "magento1":
		return fmt.Errorf("fresh install not supported for %s (use openmage instead)", config.Framework)
	case "openmage":
		return runBootstrapOpenMageFreshInstall(cmd, config, opts, cwd)
	case "symfony":
		return runBootstrapSymfonyFreshInstall(cmd, config, opts, cwd)
	case "laravel":
		return runBootstrapLaravelFreshInstall(cmd, config, opts, cwd)
	case "drupal":
		return runBootstrapDrupalFreshInstall(cmd, config, opts, cwd)
	case "wordpress":
		return runBootstrapWordPressFreshInstall(cmd, config, opts, cwd)
	case "nextjs":
		return runBootstrapNextJSFreshInstall(cmd, config, opts, cwd)
	case "emdash":
		return runBootstrapEmdashFreshInstall(cmd, config, opts, cwd)
	case "shopware":
		return runBootstrapShopwareFreshInstall(cmd, config, opts, cwd)
	case "cakephp":
		return runBootstrapCakePHPFreshInstall(cmd, config, opts, cwd)
	default:
		return fmt.Errorf("fresh install not supported for framework: %s", config.Framework)
	}
}

func runBootstrapMagentoFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	return runBootstrapFreshInstall(cmd, config, opts)
}

func runBootstrapMageOSFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	return runBootstrapFreshInstall(cmd, config, opts)
}

func runBootstrapSymfonyFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	symfonyOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
		DBHost: conventions.DefaultDBHost, // Internal container hostname
		DBUser: localDB.Username,
		DBPass: localDB.Password,
		DBName: localDB.Database,
	}

	symfonyBootstrap := bootstrap.NewSymfonyBootstrap(symfonyOpts)

	if err := symfonyBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := symfonyBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh Symfony bootstrap completed.")
	return nil
}

func runBootstrapLaravelFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	laravelOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
		DBHost: conventions.DefaultDBHost, // Internal container hostname
		DBUser: localDB.Username,
		DBPass: localDB.Password,
		DBName: localDB.Database,
	}

	laravelBootstrap := bootstrap.NewLaravelBootstrap(laravelOpts)

	if err := laravelBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := laravelBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh Laravel bootstrap completed.")
	return nil
}

func runBootstrapDrupalFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	drupalOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}

	drupalBootstrap := bootstrap.NewDrupalBootstrap(drupalOpts)

	if err := drupalBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := drupalBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh Drupal bootstrap completed.")
	return nil
}

func runBootstrapWordPressFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	wpOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
		DBHost: conventions.DefaultDBHost,
		DBUser: localDB.Username,
		DBPass: localDB.Password,
		DBName: localDB.Database,
		Domain: config.Domain,
	}

	wpBootstrap := bootstrap.NewWordPressBootstrap(wpOpts)

	if err := wpBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := wpBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh WordPress bootstrap completed.")
	return nil
}

func runBootstrapShopwareFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	shopwareOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Domain:  config.Domain,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}

	shopwareBootstrap := bootstrap.NewShopwareBootstrap(shopwareOpts)

	if err := shopwareBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := shopwareBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh Shopware bootstrap completed.")
	return nil
}

func runBootstrapCakePHPFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	cakePHPOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}

	cakePHPBootstrap := bootstrap.NewCakePHPBootstrap(cakePHPOpts)

	if err := cakePHPBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := cakePHPBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Println("Fresh CakePHP bootstrap completed.")
	return nil
}

func runBootstrapOpenMageFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	openmageOpts := bootstrap.Options{
		Version:     opts.MetaVersion,
		Env:         opts.Source,
		TablePrefix: config.TablePrefix,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}

	openmageBootstrap := bootstrap.NewOpenMageBootstrap(openmageOpts)

	if err := openmageBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := openmageBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := openmageBootstrap.Configure(cwd); err != nil {
		return fmt.Errorf("configure OpenMage: %w", err)
	}

	pterm.Success.Println("Fresh OpenMage bootstrap completed.")
	return nil
}

func runBootstrapNextJSFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	nextJSOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			// For Next.js, we might want to run commands in the web container if it's node-based
			// but for now let's keep it consistent or handle it specifically if needed.
			// Next.js currently uses CreateProject and Configure which run on host in the Next.js bootstrap.
			// I'll leave it for now or fix it if I see it's also host-only.
			return nil
		},
	}

	nextJSBootstrap := bootstrap.NewNextJSBootstrap(nextJSOpts)

	if err := nextJSBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := nextJSBootstrap.Configure(cwd); err != nil {
		return err
	}

	pterm.Success.Println("Fresh Next.js bootstrap completed.")
	return nil
}

func runBootstrapEmdashFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	emdashOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
	}

	emdashBootstrap := bootstrap.NewEmdashBootstrap(emdashOpts)

	if err := emdashBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if err := emdashBootstrap.Install(cwd); err != nil {
		return err
	}

	if err := emdashBootstrap.Configure(cwd); err != nil {
		return err
	}

	pterm.Success.Println("Fresh Emdash bootstrap completed.")
	return nil
}

func runBootstrapFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	if err := ensureBootstrapAuthJSON(config, opts); err != nil {
		return err
	}

	if err := FixComposerCompatibility(config); err != nil {
		return err
	}

	if config.Framework == "wordpress" {
		if err := FixWordPressCompatibility(config); err != nil {
			return err
		}
	}

	if err := runBootstrapFreshCreateProject(cmd, config, opts); err != nil {
		return err
	}
	if opts.HyvaInstall {
		if err := runBootstrapHyvaInstall(cmd, opts); err != nil {
			return err
		}
	}

	if err := runBootstrapPostInstall(cmd, config, opts); err != nil {
		return err
	}
	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("magento configure failed: %w", err)
	}
	if opts.IncludeSample {
		if err := runBootstrapSampleData(cmd); err != nil {
			return err
		}
	}

	pterm.Success.Println("Fresh Magento bootstrap completed.")
	return nil
}

func runBootstrapFreshCreateProject(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	commandLine := bootstrapFreshCreateProjectCommandLine(config, opts.MetaPackage, opts.MetaVersion)

	if err := runPHPContainerShellCommand(config, commandLine); err != nil {
		return fmt.Errorf("fresh create-project failed: %w", err)
	}
	return nil
}

// bootstrapFreshCreateProjectCommandLine builds the shell command for a
// fresh composer create-project, using Mage-OS's public repository for
// framework "mageos" and Magento's private repository for everything else
// (unchanged default behavior).
func bootstrapFreshCreateProjectCommandLine(config engine.Config, metaPackage string, metaVersion string) string {
	repositoryURL := "https://repo.magento.com"
	if config.Framework == "mageos" {
		repositoryURL = "https://repo.mage-os.org"
	}

	versionPart := ""
	if metaVersion != "" {
		versionPart = " " + engine.ShellQuote(metaVersion)
	}
	return strings.Join([]string{
		"set -e",
		"rm -rf /tmp/govard-create-project",
		"composer create-project -n --ignore-platform-reqs --repository-url=" + repositoryURL + " " +
			engine.ShellQuote(metaPackage) + " /tmp/govard-create-project" + versionPart,
		"if command -v rsync >/dev/null 2>&1; then rsync -a /tmp/govard-create-project/ " + conventions.DefaultWorkDir + "/; else cp -a /tmp/govard-create-project/. " + conventions.DefaultWorkDir + "/; fi",
		"rm -rf /tmp/govard-create-project",
	}, " && ")
}

// RunBootstrapFreshCreateProjectForTest exposes runBootstrapFreshCreateProject for tests in /tests.
func RunBootstrapFreshCreateProjectForTest(cmd *cobra.Command, config engine.Config, metaPackage, metaVersion string) error {
	return runBootstrapFreshCreateProject(cmd, config, BootstrapRuntimeOptions{
		MetaPackage: strings.TrimSpace(metaPackage),
		MetaVersion: strings.TrimSpace(metaVersion),
	})
}

// RunBootstrapFreshCreateProjectCommandLineForTest exposes
// bootstrapFreshCreateProjectCommandLine for tests in /tests.
func RunBootstrapFreshCreateProjectCommandLineForTest(config engine.Config, metaPackage string, metaVersion string) string {
	return bootstrapFreshCreateProjectCommandLine(config, metaPackage, metaVersion)
}

// RunBootstrapFrameworkFreshInstallForTest exposes runBootstrapFrameworkFreshInstall for tests in /tests.
func RunBootstrapFrameworkFreshInstallForTest(cmd *cobra.Command, config engine.Config, source, metaVersion string) error {
	return runBootstrapFrameworkFreshInstall(cmd, config, BootstrapRuntimeOptions{
		Source:      strings.TrimSpace(source),
		MetaVersion: strings.TrimSpace(metaVersion),
	})
}
