package cmd

import (
	"context"
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

	def, ok := frameworks.Get(config.Framework)
	if !ok || def.FreshInstall == nil {
		return fmt.Errorf("fresh install not supported for framework: %s", config.Framework)
	}
	if opts.MetaPackage == defaultBootstrapMetaPackage && def.DefaultFreshMetaPackage != "" {
		opts.MetaPackage = def.DefaultFreshMetaPackage
	}
	return runBootstrapRegistryFreshInstall(cmd, config, opts, def, cwd)
}

// runBootstrapRegistryFreshInstall dispatches to a framework's own
// FreshInstall function (internal/frameworks/<name>/freshinstall.go)
// instead of a per-framework case here. Every registered framework sets
// FreshInstall now - even Magento 1, whose FreshInstall just returns the
// "use openmage instead" error - so runBootstrapFrameworkFreshInstall has
// no framework-name switch left at all.
func runBootstrapRegistryFreshInstall(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions, def types.FrameworkDefinition, cwd string) error {
	fwOpts := bootstrap.Options{
		Version:       opts.MetaVersion,
		Env:           opts.Source,
		SkipUp:        opts.SkipUp,
		ProjectName:   config.ProjectName,
		TablePrefix:   config.TablePrefix,
		MetaPackage:   opts.MetaPackage,
		HyvaInstall:   opts.HyvaInstall,
		IncludeSample: opts.IncludeSample,
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
		EnsureAuthJSON: func() error {
			return ensureBootstrapAuthJSON(config, opts)
		},
		FixComposerCompatibility: func() error {
			return FixComposerCompatibility(config)
		},
		RunHyvaInstall: func() error {
			return runBootstrapHyvaInstall(cmd, opts)
		},
		ResolveFrameworkTablePrefix: func() (string, error) {
			if def.ResolveBootstrapTablePrefix == nil {
				return "", fmt.Errorf("fresh install table prefix is not supported for framework: %s", config.Framework)
			}
			return def.ResolveBootstrapTablePrefix(config.TablePrefix)
		},
		RunTool: func(tool string, args []string) error {
			return runGovardSubcommand(cmd, govardToolSubcommandArgs(tool, args...)...)
		},
		RunEnvironmentCommand: func(args []string) error { return runGovardSubcommand(cmd, append([]string{"env"}, args...)...) },
		IsPHPContainerRunning: func() bool {
			return engine.IsContainerRunning(context.Background(), fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix))
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
// post-fresh-install `env up` in bootstrapCmd.RunE would be redundant. See
// FrameworkDefinition.FreshInstallManagesOwnEnvUp for why Django needs this.
func frameworkFreshInstallManagesOwnEnvUp(framework string) bool {
	def, ok := frameworks.Get(framework)
	return ok && def.FreshInstallManagesOwnEnvUp
}

// FrameworkFreshInstallManagesOwnEnvUpForTest exposes frameworkFreshInstallManagesOwnEnvUp for tests in /tests.
func FrameworkFreshInstallManagesOwnEnvUpForTest(framework string) bool {
	return frameworkFreshInstallManagesOwnEnvUp(framework)
}
