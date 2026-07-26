package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runBootstrapFrameworkFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()

	if config.Framework == "mageos" && opts.MetaPackage == defaultBootstrapMetaPackage {
		opts.MetaPackage = "mage-os/project-community-edition"
	}

	if def, ok := frameworks.Get(config.Framework); ok && def.FreshInstall != nil {
		return runBootstrapRegistryFreshInstall(cmd, config, opts, def, cwd)
	}

	switch config.Framework {
	case "magento2", "mageos":
		return runBootstrapFreshInstall(cmd, config, opts)
	case "magento1":
		return fmt.Errorf("fresh install not supported for %s (use openmage instead)", config.Framework)
	default:
		return fmt.Errorf("fresh install not supported for framework: %s", config.Framework)
	}
}

// runBootstrapRegistryFreshInstall dispatches to a framework's own
// FreshInstall function (internal/frameworks/<name>/freshinstall.go)
// instead of a per-framework case here. Frameworks not yet migrated to
// this registry field (def.FreshInstall == nil) keep dispatching through
// the switch above.
func runBootstrapRegistryFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, def types.FrameworkDefinition, cwd string) error {
	fwOpts := bootstrap.Options{
		Version:     opts.MetaVersion,
		Env:         opts.Source,
		SkipUp:      opts.SkipUp,
		ProjectName: config.ProjectName,
		TablePrefix: config.TablePrefix,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}
	if def.FreshInstallNeedsDB {
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
		localDB := resolveLocalDBCredentials(config, containerName)
		fwOpts.DBHost = conventions.DefaultDBHost
		fwOpts.DBUser = localDB.Username
		fwOpts.DBPass = localDB.Password
		fwOpts.DBName = localDB.Database
	}
	if def.FreshInstallNeedsDomain {
		fwOpts.Domain = config.Domain
	}

	helpers := bootstrap.CmdHelpers{
		ConfigureAuto: func() error {
			return runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...)
		},
		NodeRunner: func(command string) error {
			return runNodeCreateProjectContainer(config, cwd, command)
		},
		PythonRunner: func(command string) error {
			return runPythonCreateProjectContainer(config, cwd, command)
		},
		EnvUp: func() error {
			return runGovardSubcommand(cmd, "env", "up", "--remove-orphans")
		},
	}

	if err := def.FreshInstall(fwOpts, cwd, helpers); err != nil {
		if errors.Is(err, bootstrap.ErrFreshInstallSkipUp) {
			return nil
		}
		return err
	}

	pterm.Success.Printf("Fresh %s bootstrap completed.\n", def.DisplayName)
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

// RunBootstrapFrameworkFreshInstallWithOptionsForTest exposes
// runBootstrapFrameworkFreshInstall for tests in /tests that need to set
// fields RunBootstrapFrameworkFreshInstallForTest doesn't forward (e.g.
// SkipUp) - added when Django moved off its own dedicated
// runBootstrapDjangoFreshInstall function onto the registry FreshInstall
// path, since RunBootstrapDjangoFreshInstallForTest (which used to serve
// this purpose) no longer has any Django-specific function left to wrap.
func RunBootstrapFrameworkFreshInstallWithOptionsForTest(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	return runBootstrapFrameworkFreshInstall(cmd, config, opts)
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
