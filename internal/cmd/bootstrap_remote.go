package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var bootstrapRemoteDirExists = func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool {
	probe := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, "test -d "+remote.QuoteRemotePath(remotePath))
	return probe.Run() == nil
}

// bootstrapPostCloneDefinition returns framework's registry entry if it
// participates in the FrameworkBootstrap.PostClone step of the remote/clone
// bootstrap workflow. magento2/mageos are excluded even though
// SupportsBootstrap is true: their post-clone setup is the
// ensureBootstrapMagentoEnvPHP branch above, not PostClone.
func bootstrapPostCloneDefinition(framework string) (types.FrameworkDefinition, bool) {
	def, ok := frameworks.Get(framework)
	if !ok || !def.SupportsBootstrap || engine.IsMagento2Family(framework) {
		return types.FrameworkDefinition{}, false
	}
	return def, true
}

func runBootstrapRemote(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	requiresRemote := opts.Clone || (opts.DBImport && opts.DBDump == "") || opts.MediaSync != ""

	if requiresRemote {
		if _, ok := config.Remotes[opts.Source]; !ok {
			if stdinIsTerminal() {
				pterm.Warning.Printf("Remote '%s' is not configured.\n", opts.Source)
				yes, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).Show(fmt.Sprintf("Would you like to add remote '%s' now?", opts.Source))
				if yes {
					if err := runGovardSubcommand(cmd, "remote", "add", opts.Source); err != nil {
						return err
					}
					// Reload config after adding remote
					newConfig, err := loadFullConfig()
					if err != nil {
						return err
					}
					config = newConfig
				} else {
					return fmt.Errorf("remote '%s' is not configured", opts.Source)
				}
			} else {
				return fmt.Errorf("remote '%s' is not configured. Add it to remotes in %s", opts.Source, conventions.BaseConfigFile)
			}
		}

		if err := runGovardSubcommand(cmd, "remote", "test", opts.Source); err != nil {
			return fmt.Errorf("remote test failed for '%s': %w", opts.Source, err)
		}
	}

	if opts.Clone {
		syncArgs := append(bootstrapFileSyncArgs(opts), "--yes")
		skipped, err := runGovardSubcommandSkippable(cmd, syncArgs...)
		if skipped {
			fmt.Println()
			pterm.Warning.Println("File sync (clone) skipped by user (SIGINT).")
		} else if err != nil {
			return fmt.Errorf("file sync failed: %w", err)
		}
	}

	cwd, _ := os.Getwd()

	if opts.ComposerInstall {
		composerJSONPath := filepath.Join(cwd, "composer.json")
		if !fileExists(composerJSONPath) {
			pterm.Info.Println("No composer.json found. Skipping composer install.")
		} else {
			// First check if composer is compatible with the current PHP version
			if err := FixComposerCompatibility(config); err != nil {
				pterm.Warning.Printf("Could not verify/fix composer compatibility: %v\n", err)
			}

			if config.Framework == "wordpress" {
				if err := FixWordPressCompatibility(config); err != nil {
					pterm.Warning.Printf("Could not verify/fix WordPress compatibility: %v\n", err)
				}
			}

			if err := ensureBootstrapAuthJSON(config, opts); err != nil {
				return err
			}
			if opts.Clone {
				if err := runBootstrapComposerPrepare(config); err != nil {
					return err
				}
			}

			if satisfied, _ := engine.VendorSatisfiesComposerLock(cwd); satisfied {
				pterm.Info.Println("vendor/ already satisfies composer.lock. Skipping composer install.")
			} else {
				skipped, installErr := runGovardSubcommandSkippable(cmd, govardComposerSubcommandArgs("install", "-n")...)
				if skipped {
					fmt.Println()
					pterm.Warning.Println("Composer install skipped by user (SIGINT).")
				} else if installErr != nil {
					autoloadPath := filepath.Join(cwd, "vendor", "autoload.php")

					// If the error specifically mentions that the container is not running, we must stop.
					errText := installErr.Error()
					if strings.Contains(errText, "not running") || strings.Contains(errText, "No such container") {
						return fmt.Errorf("composer install failed because the container is not running. Please check 'govard status' and 'docker ps': %w", installErr)
					}

					if fileExists(autoloadPath) {
						fmt.Println()
						pterm.Warning.Printf("composer install failed, but %s exists. Continuing bootstrap (%v).\n", autoloadPath, installErr)
					} else {
						fmt.Println()
						pterm.Warning.Printf("composer install failed (%v). Attempting to sync vendor from remote '%s'...\n", installErr, opts.Source)
						if err := runGovardSubcommand(cmd, "sync", "--source", opts.Source, "--file", "--path", "vendor/", "--yes"); err != nil {
							return fmt.Errorf("composer install failed (%v) and vendor sync failed (%v)", installErr, err)
						}
					}
				}
			}
		}
	}

	// Always try to re-generate autoload if a PHP project is present. This avoids runtime issues when vendor came from
	// a remote sync or when a lock file references a missing VCS commit but the dependency already exists locally.
	if opts.ComposerInstall {
		composerJSONPath := filepath.Join(cwd, "composer.json")
		if fileExists(composerJSONPath) || strings.ToLower(config.Framework) != "wordpress" {
			if err := bootstrapComposerDumpAutoload(cmd, cwd); err != nil {
				return err
			}
		}
	}

	if opts.DBImport {
		if err := runBootstrapDatabaseSync(cmd, opts); err != nil {
			return err
		}
	}

	if engine.IsMagento2Family(config.Framework) {
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

	if shouldRunFrameworkPostClone(config, opts) {
		cwd, _ := os.Getwd()
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
		localDB := resolveLocalDBCredentials(config, containerName)

		bootstrapOpts := bootstrap.Options{
			Version: opts.MetaVersion,
			Env:     opts.Source,
			Runner: func(command string) error {
				return runPHPContainerShellCommand(config, command)
			},
			DBHost:      conventions.DefaultDBHost,
			DBUser:      localDB.Username,
			DBPass:      localDB.Password,
			DBName:      localDB.Database,
			TablePrefix: config.TablePrefix,
			ProjectName: config.ProjectName,
			Domain:      config.Domain,
		}

		if config.Framework == "prestashop" {
			if remoteCfg, ok := config.Remotes[opts.Source]; ok {
				if psEnv, err := remote.ProbePrestaShopEnvironment(opts.Source, remoteCfg); err == nil {
					// The remote's actual table prefix reflects the DB that was just
					// imported and takes priority over local config, same precedence as
					// resolveRemoteDBCredentials uses for every other framework.
					if remotePrefix := engine.SafeTablePrefix(psEnv.DB.TablePrefix); remotePrefix != "" {
						bootstrapOpts.TablePrefix = remotePrefix
					}
					bootstrapOpts.PrestaShopSecret = psEnv.Secrets.Secret
					bootstrapOpts.PrestaShopCookieKey = psEnv.Secrets.CookieKey
					bootstrapOpts.PrestaShopCookieIV = psEnv.Secrets.CookieIV
					bootstrapOpts.PrestaShopNewCookieKey = psEnv.Secrets.NewCookieKey
				} else {
					pterm.Warning.Printf("Could not probe remote PrestaShop secrets/table prefix, falling back to local config: %v\n", err)
				}
			}
		}

		var frameworkBootstrap bootstrap.FrameworkBootstrap
		if def, ok := bootstrapPostCloneDefinition(config.Framework); ok {
			frameworkBootstrap = def.Bootstrap(bootstrapOpts)
		}

		if frameworkBootstrap != nil {
			if err := frameworkBootstrap.PostClone(cwd); err != nil {
				if shouldIgnoreFrameworkPostCloneError(config, err, cwd) {
					pterm.Warning.Printf("Skipping strict %s post-clone step: %v\n", config.Framework, err)
				} else {
					return err
				}
			}
		}
	} else if _, ok := bootstrapPostCloneDefinition(config.Framework); ok {
		pterm.Info.Printf("Skipping %s post-clone setup because composer install is disabled.\n", config.Framework)
	}

	if opts.AdminCreate && engine.IsMagento2Family(config.Framework) {
		runBootstrapAdminCreate(cmd, config)
	}

	if engine.IsMagento2Family(config.Framework) {
		if err := runBootstrapMagentoReindex(cmd); err != nil {
			return err
		}
	}

	if opts.MediaSync != "" {
		if skip, reason := shouldSkipBootstrapMediaSync(config, opts); skip {
			pterm.Warning.Printf("Skipping media sync: %s\n", reason)
		} else {
			args := []string{"sync", "--source", opts.Source, "--media", opts.MediaSync}
			if opts.NoNoise {
				args = append(args, "--no-noise")
			}
			for _, pattern := range opts.ExcludePatterns {
				args = append(args, "--exclude", pattern)
			}
			skipped, err := runGovardSubcommandSkippable(cmd, append(args, "--yes")...)
			if skipped {
				fmt.Println()
				pterm.Warning.Println("Media sync skipped by user (SIGINT).")
			} else if err != nil {
				fmt.Println()
				pterm.Warning.Printf("Media synchronization was not fully completed, but bootstrap will continue: %v\n", err)
			}
		}
	}

	pterm.Success.Printf("Bootstrap from remote '%s' completed.\n", opts.Source)
	return nil
}

func runBootstrapDatabaseSync(cmd *cobra.Command, opts BootstrapRuntimeOptions) error {
	if opts.DBDump != "" {
		if err := runGovardSubcommand(cmd, "db", "import", "--yes", "--file", opts.DBDump); err != nil {
			return fmt.Errorf("database import from file failed: %w", err)
		}
		return nil
	}

	if opts.StreamDB {
		importArgs := []string{"db", "import", "--yes", "--stream-db", "--environment", opts.Source}
		if opts.NoNoise {
			importArgs = append(importArgs, "--no-noise")
		}
		if opts.NoPII {
			importArgs = append(importArgs, "--no-pii")
		}
		skipped, err := runGovardSubcommandSkippable(cmd, importArgs...)
		if skipped {
			fmt.Println()
			pterm.Warning.Println("Stream-DB import skipped by user (SIGINT).")
			return nil
		} else if err != nil {
			return fmt.Errorf("stream-db import failed: %w", err)
		}
		return nil
	}

	args := []string{"sync", "--source", opts.Source, "--db"}
	if opts.NoNoise {
		args = append(args, "--no-noise")
	}
	if opts.NoPII {
		args = append(args, "--no-pii")
	}
	skipped, err := runGovardSubcommandSkippable(cmd, append(args, "--yes")...)
	if skipped {
		fmt.Println()
		pterm.Warning.Println("Database sync skipped by user (SIGINT).")
		return nil
	} else if err != nil {
		return fmt.Errorf("database sync failed: %w", err)
	}
	return nil
}

func bootstrapFileSyncArgs(opts BootstrapRuntimeOptions) []string {
	args := []string{
		"sync",
		"--source", opts.Source,
		"--file",
	}

	if opts.NoNoise {
		args = append(args, "--no-noise")
	}
	if opts.DeleteSync {
		args = append(args, "--delete")
	}
	if opts.NoCompress {
		args = append(args, "--no-compress")
	}
	for _, pattern := range opts.ExcludePatterns {
		args = append(args, "--exclude", pattern)
	}

	// Default excludes for bootstrap (to protect local config)
	args = append(args,
		"--exclude", ".git",
		"--exclude", ".env",
		"--exclude", ".idea",
		"--exclude", "auth.json",
		"--exclude", "app/etc/env.php",
		"--exclude", "app/etc/local.xml",
		"--exclude", "generated",
		"--exclude", "node_modules",
		"--exclude", "pub/static",
		"--exclude", "pub/media",
		"--exclude", "media",
		"--exclude", "var",
	)
	return args
}

func SetBootstrapRemoteDirExistsForTest(fn func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool) func() {
	previous := bootstrapRemoteDirExists
	bootstrapRemoteDirExists = fn
	return func() {
		bootstrapRemoteDirExists = previous
	}
}

// RunBootstrapRemoteForTest exposes runBootstrapRemote for tests in /tests.
func RunBootstrapRemoteForTest(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	return runBootstrapRemote(cmd, config, opts)
}
