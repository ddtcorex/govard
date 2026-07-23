package cmd

import (
	"fmt"
	"os"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// genericFreshInstallFrameworks lists frameworks whose fresh-install is a
// uniform CreateProject -> Install -> `govard config auto` sequence,
// differing only in which bootstrap.Options fields they need - handled by
// runBootstrapGenericFreshInstall. Frameworks with a materially different
// sequence (openmage, nextjs, emdash call Configure directly instead of
// shelling out; magento2/mageos/magento1 have their own elaborate/blocked
// paths) are NOT here and keep their own function.
var genericFreshInstallFrameworks = map[string]struct{ needsDB, needsDomain bool }{
	"symfony":   {needsDB: true},
	"laravel":   {needsDB: true},
	"drupal":    {},
	"wordpress": {needsDB: true, needsDomain: true},
	"shopware":  {needsDomain: true},
	"cakephp":   {},
}

func runBootstrapFrameworkFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()

	if config.Framework == "mageos" && opts.MetaPackage == defaultBootstrapMetaPackage {
		opts.MetaPackage = "mage-os/project-community-edition"
	}

	if needs, ok := genericFreshInstallFrameworks[config.Framework]; ok {
		return runBootstrapGenericFreshInstall(cmd, config, opts, cwd, needs.needsDB, needs.needsDomain)
	}

	switch config.Framework {
	case "magento2", "mageos":
		return runBootstrapFreshInstall(cmd, config, opts)
	case "magento1":
		return fmt.Errorf("fresh install not supported for %s (use openmage instead)", config.Framework)
	case "openmage":
		return runBootstrapOpenMageFreshInstall(cmd, config, opts, cwd)
	case "nextjs":
		return runBootstrapNextJSFreshInstall(cmd, config, opts, cwd)
	case "emdash":
		return runBootstrapEmdashFreshInstall(cmd, config, opts, cwd)
	case "django":
		return runBootstrapDjangoFreshInstall(cmd, config, opts, cwd)
	default:
		return fmt.Errorf("fresh install not supported for framework: %s", config.Framework)
	}
}

// runBootstrapGenericFreshInstall runs the CreateProject -> Install ->
// `govard config auto` sequence shared by genericFreshInstallFrameworks.
func runBootstrapGenericFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string, needsDB bool, needsDomain bool) error {
	fwOpts := bootstrap.Options{
		Version: opts.MetaVersion,
		Env:     opts.Source,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}
	if needsDB {
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
		localDB := resolveLocalDBCredentials(config, containerName)
		fwOpts.DBHost = conventions.DefaultDBHost // Internal container hostname
		fwOpts.DBUser = localDB.Username
		fwOpts.DBPass = localDB.Password
		fwOpts.DBName = localDB.Database
	}
	if needsDomain {
		fwOpts.Domain = config.Domain
	}

	def, ok := frameworks.Get(config.Framework)
	if !ok || def.Bootstrap == nil {
		return fmt.Errorf("fresh install not supported for framework: %s", config.Framework)
	}
	fwBootstrap := def.Bootstrap(fwOpts)

	if err := fwBootstrap.CreateProject(cwd); err != nil {
		return err
	}
	if err := fwBootstrap.Install(cwd); err != nil {
		return err
	}
	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	pterm.Success.Printf("Fresh %s bootstrap completed.\n", def.DisplayName)
	return nil
}

func runBootstrapOpenMageFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	openmageOpts := bootstrap.Options{
		Version:     opts.MetaVersion,
		Env:         opts.Source,
		TablePrefix: config.TablePrefix,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
		DBHost:      conventions.DefaultDBHost,
		DBUser:      localDB.Username,
		DBPass:      localDB.Password,
		DBName:      localDB.Database,
		ProjectName: config.ProjectName,
		Domain:      config.Domain,
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
			return runNodeCreateProjectContainer(config, cwd, command)
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

func runBootstrapDjangoFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, cwd string) error {
	djangoOpts := bootstrap.Options{
		Version:     opts.MetaVersion,
		Env:         opts.Source,
		ProjectName: config.ProjectName,
		Domain:      config.Domain,
		Runner: func(command string) error {
			return runPythonCreateProjectContainer(config, cwd, command)
		},
	}

	djangoBootstrap := bootstrap.NewDjangoBootstrap(djangoOpts)

	if err := djangoBootstrap.CreateProject(cwd); err != nil {
		return err
	}

	if opts.SkipUp {
		pterm.Info.Println("Skipping env up and migrate (--no-up); run `govard env up` then `govard tool manage migrate` manually.")
		return nil
	}

	if err := runGovardSubcommand(cmd, "env", "up", "--remove-orphans"); err != nil {
		return fmt.Errorf("failed to start local environment: %w", err)
	}

	if err := djangoBootstrap.Install(cwd); err != nil {
		return err
	}

	pterm.Success.Println("Fresh Django bootstrap completed.")
	return nil
}

// RunBootstrapDjangoFreshInstallForTest exposes runBootstrapDjangoFreshInstall
// for tests in /tests, since it needs opts.SkipUp which
// RunBootstrapFrameworkFreshInstallForTest doesn't forward.
func RunBootstrapDjangoFreshInstallForTest(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()
	return runBootstrapDjangoFreshInstall(cmd, config, opts, cwd)
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
		return fmt.Errorf("framework configuration failed: %w", err)
	}
	if opts.IncludeSample {
		if err := runBootstrapSampleData(cmd); err != nil {
			return err
		}
	}

	pterm.Success.Printf("Fresh %s bootstrap completed.\n", engine.Magento2FamilyDisplayName(config.Framework))
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

// frameworkFreshInstallManagesOwnEnvUp reports whether the framework's
// fresh-install function already calls `env up` itself (and, if needed,
// runs Install()/migrate against the running containers) - so the generic
// post-fresh-install `env up` in bootstrapCmd.RunE would be redundant.
// Django's compose "web" container executes `python manage.py runserver`
// directly and can't come up against an empty project directory, so its
// fresh-install path must scaffold the project first, then bring the
// environment up itself before running Install() - by the time control
// returns to RunE, env up has already happened.
func frameworkFreshInstallManagesOwnEnvUp(framework string) bool {
	return framework == "django"
}

// FrameworkFreshInstallManagesOwnEnvUpForTest exposes frameworkFreshInstallManagesOwnEnvUp for tests in /tests.
func FrameworkFreshInstallManagesOwnEnvUpForTest(framework string) bool {
	return frameworkFreshInstallManagesOwnEnvUp(framework)
}
