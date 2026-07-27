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

	if config.Framework == "mageos" && opts.MetaPackage == defaultBootstrapMetaPackage {
		opts.MetaPackage = "mage-os/project-community-edition"
	}

	if def, ok := frameworks.Get(config.Framework); ok && def.FreshInstall != nil {
		return runBootstrapRegistryFreshInstall(cmd, config, opts, def, cwd)
	}

	switch config.Framework {
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
		ResolveMagentoTablePrefix: func() (string, error) {
			return resolveBootstrapMagentoTablePrefix(config)
		},
		RunMagentoSetupInstall: func(args []string) error {
			return runBootstrapMagentoSetupInstall(cmd, config, args)
		},
		RunMagentoSampleData: func() error {
			return runBootstrapSampleData(cmd)
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

// runBootstrapMagentoSetupInstall runs `bin/magento setup:install` with the
// given args, first applying a best-effort Elasticsearch/OpenSearch
// read-only-allow-delete unblock if the PHP container is already running.
// Moved from the tail of runBootstrapPostInstall (internal/cmd/
// bootstrap_post_install.go) - the "build the args" half of that function
// is now bootstrap.BuildMagentoSetupInstallArgs
// (internal/engine/bootstrap/magento_family.go), called by the Magento
// family's shared freshInstall before this function ever runs.
func runBootstrapMagentoSetupInstall(cmd *cobra.Command, config engine.Config, args []string) error {
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

	if err := runGovardSubcommand(cmd, govardMagentoSubcommandArgs(args...)...); err != nil {
		return fmt.Errorf("magento setup:install failed: %w", err)
	}
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
