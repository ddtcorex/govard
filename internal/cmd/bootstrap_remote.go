package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var bootstrapRemoteDirExists = func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool {
	probe := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, "test -d "+shellQuote(remotePath))
	return probe.Run() == nil
}

func runBootstrapRemote(cmd *cobra.Command, config engine.Config, opts bootstrapRuntimeOptions) error {
	requiresRemote := opts.Clone || (opts.DBImport && opts.DBDump == "") || opts.MediaSync

	if requiresRemote {
		if _, ok := config.Remotes[opts.Source]; !ok {
			return fmt.Errorf("remote '%s' is not configured. Add it to remotes in %s", opts.Source, engine.BaseConfigFile)
		}

		if err := runGovardSubcommand(cmd, "remote", "test", opts.Source); err != nil {
			return fmt.Errorf("remote test failed for '%s': %w", opts.Source, err)
		}
	}

	if opts.Clone {
		if err := runGovardSubcommand(cmd, bootstrapFileSyncArgs(opts)...); err != nil {
			return fmt.Errorf("file sync failed: %w", err)
		}
	}

	cwd, _ := os.Getwd()

	if opts.ComposerInstall {
		if err := ensureBootstrapAuthJSON(config, opts); err != nil {
			return err
		}
		if opts.Clone {
			if err := runBootstrapComposerPrepare(config); err != nil {
				return err
			}
		}

		installErr := runGovardSubcommand(cmd, govardComposerSubcommandArgs("install", "-n")...)
		if installErr != nil {
			autoloadPath := filepath.Join(cwd, "vendor", "autoload.php")
			if fileExists(autoloadPath) {
				pterm.Warning.Printf("composer install failed, but %s exists. Continuing bootstrap (%v).\n", autoloadPath, installErr)
			} else {
				pterm.Warning.Printf("composer install failed (%v). Attempting to sync vendor from remote '%s'...\n", installErr, opts.Source)
				if err := runGovardSubcommand(cmd, "sync", "--source", opts.Source, "--file", "--path", "vendor"); err != nil {
					return fmt.Errorf("composer install failed (%v) and vendor sync failed (%v)", installErr, err)
				}
			}
		}
	}

	// Always try to re-generate autoload if a PHP project is present. This avoids runtime issues when vendor came from
	// a remote sync or when a lock file references a missing VCS commit but the dependency already exists locally.
	if opts.ComposerInstall {
		if err := bootstrapComposerDumpAutoload(cmd, cwd); err != nil {
			return err
		}
	}

	if opts.DBImport {
		if err := runBootstrapDatabaseSync(cmd, opts); err != nil {
			return err
		}
	}

	if config.Framework == "magento2" {
		if err := ensureBootstrapMagentoEnvPHP(config, opts); err != nil {
			return err
		}
	}

	if !opts.SkipUp {
		if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
			return fmt.Errorf("configure failed: %w", err)
		}
	}

	// Some Magento commands can invalidate generated classes that were previously indexed in classmaps.
	// Rebuild autoload once more so subsequent steps (admin user, smoke checks) do not fail on stale references.
	if opts.ComposerInstall {
		if err := bootstrapComposerDumpAutoload(cmd, cwd); err != nil {
			return err
		}
	}

	if shouldRunSymfonyPostClone(config, opts) {
		cwd, _ := os.Getwd()
		symfonyOpts := bootstrap.Options{
			Version: opts.MetaVersion,
			Env:     opts.Source,
		}
		symfonyBootstrap := bootstrap.NewSymfonyBootstrap(symfonyOpts)
		if err := symfonyBootstrap.PostClone(cwd); err != nil {
			if shouldIgnoreSymfonyPostCloneError(err, cwd) {
				pterm.Warning.Printf("Skipping strict Symfony post-clone step: %v\n", err)
			} else {
				return err
			}
		}
	} else if config.Framework == "symfony" {
		pterm.Info.Println("Skipping Symfony post-clone setup because composer install is disabled.")
	}

	if opts.AdminCreate && config.Framework == "magento2" {
		runBootstrapAdminCreate(cmd, config)
	}

	if config.Framework == "magento2" {
		if err := runBootstrapMagentoReindex(cmd); err != nil {
			return err
		}
	}

	if opts.MediaSync {
		if skip, reason := shouldSkipBootstrapMediaSync(config, opts); skip {
			pterm.Warning.Printf("Skipping media sync: %s\n", reason)
		} else {
			args := []string{"sync", "--source", opts.Source, "--media"}
			if config.Framework == "magento2" {
				args = append(args, bootstrapMagentoMediaSyncArgs(opts)...)
			}
			if err := runGovardSubcommand(cmd, args...); err != nil {
				return fmt.Errorf("media sync failed: %w", err)
			}
		}
	}

	pterm.Success.Printf("Bootstrap from remote '%s' completed.\n", opts.Source)
	return nil
}

func runBootstrapDatabaseSync(cmd *cobra.Command, opts bootstrapRuntimeOptions) error {
	if opts.DBDump != "" {
		if err := runGovardSubcommand(cmd, "db", "import", "--file", opts.DBDump); err != nil {
			return fmt.Errorf("database import from file failed: %w", err)
		}
		return nil
	}

	if opts.StreamDB {
		if err := runGovardSubcommand(cmd, "db", "import", "--stream-db", "--environment", opts.Source); err != nil {
			return fmt.Errorf("stream-db import failed: %w", err)
		}
		return nil
	}

	if err := runGovardSubcommand(cmd, "sync", "--source", opts.Source, "--db"); err != nil {
		return fmt.Errorf("database sync failed: %w", err)
	}
	return nil
}

func bootstrapFileSyncArgs(opts bootstrapRuntimeOptions) []string {
	args := []string{
		"sync",
		"--source", opts.Source,
		"--file",
		"--exclude", ".git",
		"--exclude", ".env",
		"--exclude", ".idea",
		"--exclude", "auth.json",
		"--exclude", "app/etc/env.php",
		"--exclude", "generated",
		"--exclude", "node_modules",
		"--exclude", "pub/static",
		"--exclude", "pub/media",
		"--exclude", "var",
	}
	return args
}

func SetBootstrapRemoteDirExistsForTest(fn func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool) func() {
	previous := bootstrapRemoteDirExists
	bootstrapRemoteDirExists = fn
	return func() {
		bootstrapRemoteDirExists = previous
	}
}
