package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/deploy"
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

var frameworkLookupForBootstrap = frameworks.Get

func prepareFrameworkComposer(config engine.Config) error {
	definition, ok := frameworkLookupForBootstrap(config.Framework)
	if !ok || definition.PrepareComposer == nil {
		return nil
	}
	return definition.PrepareComposer(config)
}

// SetFrameworkLookupForBootstrapForTest swaps the registry lookup used by the
// clone-bootstrap capabilities. It lets tests exercise a hook without running
// an external framework command.
func SetFrameworkLookupForBootstrapForTest(fn func(name string) (types.FrameworkDefinition, bool)) func() {
	previous := frameworkLookupForBootstrap
	frameworkLookupForBootstrap = fn
	return func() { frameworkLookupForBootstrap = previous }
}

// PrepareFrameworkComposerForTest exposes the generic composer preparation
// dispatch for a focused capability test.
func PrepareFrameworkComposerForTest(config engine.Config) error {
	return prepareFrameworkComposer(config)
}

func shouldRunComposerDumpAutoload(framework string, composerJSONExists bool) bool {
	if composerJSONExists {
		return true
	}
	definition, ok := frameworkLookupForBootstrap(framework)
	return !ok || !definition.RequiresComposerManifestForDumpAutoload
}

// ShouldRunComposerDumpAutoloadForTest exposes the registry-driven policy for
// a focused regression test.
func ShouldRunComposerDumpAutoloadForTest(framework string, composerJSONExists bool) bool {
	return shouldRunComposerDumpAutoload(framework, composerJSONExists)
}

// bootstrapPostCloneDefinition returns framework's registry entry if it
// participates in the generic FrameworkBootstrap.PostClone interface step of
// the remote/clone bootstrap workflow. magento2/mageos are excluded even
// though SupportsBootstrap is true: their real pre-configure/post-clone
// setup is dispatched separately in runBootstrapRemote via Definition()'s
// PreConfigureHook/PostCloneHook fields, not through this interface method.
func bootstrapPostCloneDefinition(framework string) (types.FrameworkDefinition, bool) {
	def, ok := frameworks.Get(framework)
	if !ok || !def.SupportsBootstrap || def.PreConfigureHook != nil || def.PostCloneHook != nil {
		return types.FrameworkDefinition{}, false
	}
	return def, true
}

func runBootstrapRemote(cmd *cobra.Command, config engine.Config, opts BootstrapRuntimeOptions) error {
	requiresRemote := opts.Clone || (opts.DBImport && opts.DBDump == "") || opts.MediaSync != ""

	if requiresRemote {
		// Resolved, never read raw: `config.Remotes[source]` is a map lookup and
		// nothing more, so it answered "not configured" for the one remote whose
		// identity is container-derived, one screen after ResolveAutoRemote had
		// already accepted the name. resolvedRemoteForName is the seam that
		// resolves the container and layers the project's `remotes.sandbox`
		// block over it, and it keeps the two failure shapes apart: a missing
		// ordinary remote is an absence the caller may act on (offer to add it),
		// while a missing sandbox is a real error that already names the remedy.
		//
		// Today the only name that resolves with an error is the synthetic one,
		// which is exactly the one `remote add` cannot create — so the two
		// conditions below always agree and the offer is unreachable for it
		// today. They are stated separately on purpose: the error is the
		// resolver's remedy, and bootstrapOffersToAddRemote is the policy of
		// which names a human prompt could fix at all, and a second erroring
		// name must not inherit the offer by accident.
		if _, ok, resolveErr := resolvedRemoteForName(cmd.Context(), config, opts.Source); !ok {
			if resolveErr != nil && !bootstrapOffersToAddRemote(opts.Source) {
				return resolveErr
			}
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
		syncArgs := append(bootstrapFileSyncArgs(config, opts), "--yes")
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

			if err := prepareFrameworkComposer(config); err != nil {
				pterm.Warning.Printf("Could not prepare framework composer compatibility: %v\n", err)
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
				guardDone := func() {}
				if def, ok := frameworks.Get(config.Framework); ok && def.ComposerInstallGuard != nil {
					guardDone = def.ComposerInstallGuard(cwd)
				}
				skipped, installErr := runGovardSubcommandSkippable(cmd, govardComposerSubcommandArgs("install", "-n")...)
				guardDone()
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
		if shouldRunComposerDumpAutoload(config.Framework, fileExists(composerJSONPath)) {
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

	// Most fields here are pre-populated for the benefit of future hooks;
	// the current two (MagentoFamilyPreConfigure/MagentoFamilyPostClone)
	// only read AdminCreate.
	hookOpts := bootstrap.Options{
		Version:     opts.MetaVersion,
		Env:         opts.Source,
		ProjectName: config.ProjectName,
		Domain:      config.Domain,
		TablePrefix: config.TablePrefix,
		AdminCreate: opts.AdminCreate,
		Runner: func(command string) error {
			return runPHPContainerShellCommand(config, command)
		},
	}
	hookHelpers := bootstrap.CmdHelpers{
		EnsureFrameworkEnvironment: func() error {
			return ensureBootstrapFrameworkEnvironment(config, opts)
		},
		RunTool: func(tool string, args []string) error {
			return runGovardSubcommand(cmd, govardToolSubcommandArgs(tool, args...)...)
		},
		RunToolSilent: func(tool string, args []string) error {
			return runGovardSubcommandSilent(cmd, govardToolSubcommandArgs(tool, args...)...)
		},
		IsPHPContainerRunning: func() bool {
			return engine.IsContainerRunning(context.Background(), fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix))
		},
	}

	if def, ok := frameworks.Get(config.Framework); ok && def.PreConfigureHook != nil {
		if err := def.PreConfigureHook(hookOpts, cwd, hookHelpers); err != nil {
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
			DBEngine:    config.Stack.Services.DB,
			DBVersion:   config.Stack.DBVersion,
			DBUser:      localDB.Username,
			DBPass:      localDB.Password,
			DBName:      localDB.Database,
			TablePrefix: config.TablePrefix,
			ProjectName: config.ProjectName,
			Domain:      config.Domain,
		}

		if def, ok := frameworks.Get(config.Framework); ok && def.ProbeRemoteBootstrapMetadata != nil {
			// Resolved, never raw: the probe SSHes to this remote, so a
			// configured `remotes.sandbox` must contribute its capabilities but
			// not its host, user or auth.
			if remoteCfg, configured, _ := resolvedRemoteForName(cmd.Context(), config, opts.Source); configured {
				metadata, err := def.ProbeRemoteBootstrapMetadata(opts.Source, remoteCfg)
				if err != nil {
					pterm.Warning.Printf("Could not probe remote bootstrap metadata, falling back to local config: %v\n", err)
				} else {
					if remotePrefix := engine.SafeTablePrefix(metadata.TablePrefix); remotePrefix != "" {
						bootstrapOpts.TablePrefix = remotePrefix
					}
					bootstrapOpts.RemoteMetadata = metadata.Private
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

	if def, ok := frameworks.Get(config.Framework); ok && def.PostCloneHook != nil {
		if opts.ComposerInstall {
			if err := def.PostCloneHook(hookOpts, cwd, hookHelpers); err != nil {
				return err
			}
		} else {
			pterm.Info.Printf("Skipping %s post-clone hook because composer install is disabled.\n", config.Framework)
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

func bootstrapFileSyncArgs(config engine.Config, opts BootstrapRuntimeOptions) []string {
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

	// Default excludes for bootstrap (to protect local config): the generic set
	// every project needs, then what the framework itself declares as local-only
	// and its media directory (media has its own sync step).
	args = append(args,
		"--exclude", ".git",
		"--exclude", ".env",
		"--exclude", ".idea",
		"--exclude", "auth.json",
		"--exclude", "node_modules",
		// A published release carries govard deploy's record in .dep/; it
		// describes the remote and has no place in a local checkout.
		"--exclude", "/.dep",
	)
	for _, pattern := range bootstrapFrameworkCloneExcludes(config.Framework) {
		args = append(args, "--exclude", pattern)
	}
	return args
}

// bootstrapFrameworkCloneExcludes returns the clone excludes the framework's
// definition declares, plus its local media directory. An unknown framework
// contributes none, so a clone never carries another framework's paths.
func bootstrapFrameworkCloneExcludes(framework string) []string {
	def, ok := frameworks.Get(strings.ToLower(strings.TrimSpace(framework)))
	if !ok {
		return nil
	}
	patterns := append([]string(nil), def.Manifest.Sync.CloneExcludes...)
	if media := strings.TrimSpace(def.Manifest.Paths.LocalMedia); media != "" {
		patterns = append(patterns, media)
	}
	return patterns
}

// bootstrapOffersToAddRemote reports whether a missing `source` is something
// `remote add` could actually create.
//
// The synthetic sandbox is the one name it cannot: `remote add sandbox` writes
// the rehearsal's shape and no identity, and the identity comes from a
// container only `govard sandbox up` creates. Offering to add it would send an
// operator through a command that succeeds, writes a block, and leaves them
// with the same missing sandbox — so the resolver's own message, which names
// `sandbox up`, is returned instead.
//
// The name is matched the way every other sandbox name check in the codebase
// matches it: case-insensitively and after trimming, because a hand-typed
// "Sandbox" is the same remote, not a second one that happens to be addable.
func bootstrapOffersToAddRemote(source string) bool {
	return !strings.EqualFold(strings.TrimSpace(source), deploy.SandboxRemoteName)
}

// BootstrapOffersToAddRemoteForTest exposes bootstrapOffersToAddRemote to the
// tests/ package. The interactive branch it guards cannot be driven from a
// non-terminal test run, so the decision is pinned where it is made.
func BootstrapOffersToAddRemoteForTest(source string) bool {
	return bootstrapOffersToAddRemote(source)
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
