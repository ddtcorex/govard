package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	defaultBootstrapMetaPackage = "magento/project-community-edition"
	defaultBootstrapHyvaToken   = "2a749843f9e64f7e5f74495baafbd7422271d23933e8d00059a3072767c0"
)

var (
	bootstrapClone            bool
	bootstrapCodeOnly         bool
	bootstrapFresh            bool
	bootstrapIncludeSample    bool
	bootstrapSkipDB           bool
	bootstrapSkipMedia        bool
	bootstrapSkipComposer     bool
	bootstrapSkipAdmin        bool
	bootstrapNoStreamDB       bool
	bootstrapEnv              string
	bootstrapFramework        string
	bootstrapFrameworkVersion string
	bootstrapSkipUp           bool
	bootstrapMetaPackage      string
	bootstrapDBDump           string
	bootstrapFixDeps          bool
	bootstrapHyvaInstall      bool
	bootstrapHyvaToken        string
	bootstrapMageUsername     string
	bootstrapMagePassword     string
	bootstrapAssumeYes        bool
	bootstrapMediaModeFlag    string
	bootstrapPlan             bool
	bootstrapNoNoise          bool
	bootstrapNoPII            bool
	bootstrapDelete           bool
	bootstrapNoCompress       bool
	bootstrapExclude          []string
)

var bootstrapCmd = &cobra.Command{
	Use:     "bootstrap [flags]",
	Aliases: []string{"boot"},
	Short:   "Bootstrap local environment: import DB/media from remote, or full clone with --clone",
	Long: `Quickly set up a local project from a remote environment or a fresh installation.
Ideal for onboarding new team members or re-initialising a local workspace.

Two primary modes:
  Default (no --clone): Starts the local environment, runs composer install, imports the
    remote database, and syncs media — but does NOT rsync the source code files. Use this
    when your source code is already checked out from Git.
  --clone: Performs a full file rsync FROM the remote before the steps above. Use this only
    when you need an exact copy of the remote source (e.g. first-time onboarding without Git).

Media Sync Modes (--media):
- optimized: Default. Skips large generated/cache and heavy assets like products (Magento).
- catalog: Includes product images but skips product caches (Magento only).
- all: Syncs everything without exclusions.
- minimal: Syncs only non-binary assets (CSS, JS, Fonts). Skips images/videos.
- none: Skips media sync entirely (same as --no-media).

Framework Specifics:
- Magento 2: Automates auth.json, env.php, database import, media sync, and admin user creation.
- Symfony/Laravel: Handles .env generation and composer install.
- WordPress: Configures wp-config.php and imports database.

Case Studies:
- Day-to-day refresh: 'govard bootstrap -e staging' — syncs DB and media, keeps your local git tree.
- Refresh without PII tables: 'govard bootstrap -e staging --no-pii' — syncs DB excluding sensitive tables.
- First-time onboarding (no git clone): 'govard bootstrap --clone -e staging' — pulls all source files.
- Fresh Start: 'govard bootstrap --framework magento2 --fresh --framework-version 2.4.8' — clean Magento install from Composer.
- Code only: 'govard bootstrap --clone --code-only -e dev' — files only, skip DB and media.
- Specify Framework: 'govard bootstrap --framework magento2' — Ensures Magento 2 environment if initialization is required.
- Minimal Frontend Sync: 'govard bootstrap -e staging --media minimal' — Syncs DB and only light assets (CSS/JS).

Note: -e/--environment accepts remote name aliases (e.g. 'dev' matches a remote named 'development').`,
	Example: `  # Refresh DB + media from dev (default — does NOT overwrite source files)
  govard bootstrap -e dev

  # Full clone (source files + DB + media) from staging
  govard bootstrap --clone -e staging

  # Clone DB excluding noise and PII tables
  govard bootstrap -e staging --no-pii

  # Fresh Magento 2.4.8 install with sample data
  govard bootstrap --framework magento2 --fresh --framework-version 2.4.8 --include-sample

  # Clone from dev but skip media sync
  govard bootstrap --clone -e dev --no-media

  # Clone source code only (skip DB and media)
  govard bootstrap --clone --code-only -e dev

  # Bootstrap and ensure Magento 2 if init runs
  govard bootstrap --framework magento2`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		cwd, _ := os.Getwd()
		configForObservability := engine.Config{}
		operationSource := ""
		defer func() {
			status := engine.OperationStatusSuccess
			message := "bootstrap completed"
			category := ""
			if err != nil {
				status = engine.OperationStatusFailure
				message = err.Error()
				category = classifyCommandError(err)
			}
			writeOperationEventBestEffort(
				"bootstrap.run",
				status,
				configForObservability,
				operationSource,
				"",
				message,
				category,
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(configForObservability, cwd, "bootstrap")
			}
		}()

		opts, err := resolveBootstrapOptions(cmd, args)
		if err != nil {
			return err
		}
		operationSource = opts.Source

		if err := ensureBootstrapInit(cmd, cwd); err != nil {
			return err
		}

		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		configForObservability = config

		supportedFrameworks := []string{"magento2", "magento1", "openmage", "laravel", "symfony", "wordpress", "prestashop"}
		if opts.Fresh {
			supportedFrameworks = []string{"magento2", "magento1", "laravel", "symfony", "openmage", "drupal", "wordpress", "nextjs", "emdash", "shopware", "cakephp"}
		}

		if !stringSliceContains(supportedFrameworks, config.Framework) {
			return fmt.Errorf("bootstrap currently supports these project types: %s (detected: %s)",
				strings.Join(supportedFrameworks, ", "), config.Framework)
		}

		var resolvedRemote string
		if needsRemoteEnvironment(opts) || opts.Source != "" {
			resolvedRemote, err = ResolveAutoRemote(config, opts.Source)
			if err != nil {
				if stdinIsTerminal() && !opts.AssumeYes && !opts.Plan {
					pterm.Warning.Printf("No remote environment found: %v\n", err)
					options := []string{"dev", "staging", "production", "qa", "preprod", "custom..."}
					selected, _ := pterm.DefaultInteractiveSelect.
						WithDefaultText("Select a remote to add").
						WithOptions(options).
						WithDefaultOption("dev").
						Show()

					remoteName := selected
					if selected == "custom..." {
						remoteName, _ = pterm.DefaultInteractiveTextInput.WithDefaultText("Enter remote name (e.g. qa)").Show()
					}
					remoteName = strings.ToLower(strings.TrimSpace(remoteName))

					if remoteName != "" {
						if err := runGovardSubcommand(cmd, "remote", "add", remoteName); err != nil {
							return fmt.Errorf("failed to add remote: %w", err)
						}
						// Reload config and try again
						config, _ = loadFullConfig()
						resolvedRemote, err = ResolveAutoRemote(config, opts.Source)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					if opts.Plan {
						// For plans, failure to resolve an auto-remote is non-fatal.
						return nil
					}
					return err
				}
			}
			opts.Source = resolvedRemote
		}
		operationSource = opts.Source

		if !opts.Fresh {
			plan, err := buildBootstrapRemotePlan(config, opts)
			if err != nil {
				return err
			}

			if opts.Plan {
				for _, line := range buildBootstrapPlanSummary(config, opts.Source, plan) {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				return nil
			}

			if !opts.AssumeYes {
				if !stdinIsTerminal() {
					return fmt.Errorf("confirmation required to proceed with bootstrap plan; use -y to assume yes in non-interactive environments")
				}
				for _, line := range buildBootstrapPlanSummary(config, opts.Source, plan) {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				fmt.Println()
				confirmed, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).WithDefaultText("Do you want to proceed with this bootstrap?").Show()
				if !confirmed {
					return fmt.Errorf("bootstrap cancelled by user")
				}
			}

			if !opts.SkipUp {
				if err := runGovardSubcommand(cmd, "env", "up", "--remove-orphans"); err != nil {
					return fmt.Errorf("failed to start local environment: %w", err)
				}
			}

			pterm.Info.Printf("Bootstrapping project from remote '%s'...\n", opts.Source)
			if err := runBootstrapRemote(cmd, config, opts); err != nil {
				return err
			}
		} else {
			startEnvBeforeFreshInstall := !opts.SkipUp && frameworkRequiresRunningEnvForFreshInstall(config.Framework)
			if startEnvBeforeFreshInstall {
				if err := runGovardSubcommand(cmd, "env", "up", "--remove-orphans"); err != nil {
					return fmt.Errorf("failed to start local environment: %w", err)
				}
			}

			pterm.Info.Printf("Bootstrapping fresh %s project...\n", config.Framework)
			if err := runBootstrapFrameworkFreshInstall(cmd, config, opts); err != nil {
				return err
			}
			if !opts.SkipUp && !startEnvBeforeFreshInstall {
				if err := runGovardSubcommand(cmd, "env", "up", "--remove-orphans"); err != nil {
					return fmt.Errorf("failed to start local environment: %w", err)
				}
			}
		}

		pterm.DefaultSection.Println("Project Information")
		pterm.Info.Printf("Project:   %s\n", config.ProjectName)
		pterm.Info.Printf("Framework: %s\n", config.Framework)
		pterm.Info.Printf("Domain:    %s\n", config.Domain)
		pterm.Info.Printf("URL:       https://%s\n", config.Domain)

		pterm.Success.Printf("Bootstrap completed in %s.\n", time.Since(startedAt).Round(time.Second))
		return nil
	},
}

func ensureBootstrapInit(cmd *cobra.Command, cwd string) error {
	configPath := filepath.Join(cwd, conventions.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check %s: %w", conventions.BaseConfigFile, err)
	}

	pterm.Info.Printf("%s not found. Running `govard init` first.\n", conventions.BaseConfigFile)
	initArgs := []string{"init"}
	if bootstrapFramework != "" {
		initArgs = append(initArgs, "--framework", bootstrapFramework)
	}
	if bootstrapFrameworkVersion != "" {
		initArgs = append(initArgs, "--framework-version", bootstrapFrameworkVersion)
	}
	if bootstrapAssumeYes {
		initArgs = append(initArgs, "--yes")
	}
	return runGovardSubcommand(cmd, initArgs...)
}

var phpContainerShellRunner = func(config engine.Config, commandLine string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	dockerArgs := []string{"exec"}
	if stdinIsTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	}

	cwd, _ := os.Getwd()
	authPath := filepath.Join(cwd, "auth.json")
	if !fileExists(authPath) {
		// Fallback to global composer auth on host if project-specific is missing
		if home, err := os.UserHomeDir(); err == nil {
			authPath = filepath.Join(home, ".composer", "auth.json")
		}
	}

	if data, err := os.ReadFile(authPath); err == nil {
		dockerArgs = append(dockerArgs, "-e", "COMPOSER_AUTH="+string(data))
	}

	if user := ResolveProjectExecUser(config, conventions.UserWWWData); strings.TrimSpace(user) != "" {
		dockerArgs = append(dockerArgs, "-u", user)
	}
	dockerArgs = append(dockerArgs, "-w", "/", containerName, "sh", "-lc", buildPHPContainerShellCommand(commandLine))
	dockerCmd := exec.Command("docker", dockerArgs...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	return dockerCmd.Run()
}

func govardComposerSubcommandArgs(args ...string) []string {
	commandArgs := []string{"tool", "composer"}
	commandArgs = append(commandArgs, args...)
	return commandArgs
}

func govardMagentoSubcommandArgs(args ...string) []string {
	commandArgs := []string{"tool", "magento"}
	commandArgs = append(commandArgs, args...)
	return commandArgs
}

func govardConfigureSubcommandArgs() []string {
	return []string{"config", "auto"}
}

func runPHPContainerShellCommand(config engine.Config, commandLine string) error {
	return phpContainerShellRunner(config, commandLine)
}

func buildPHPContainerShellCommand(commandLine string) string {
	return "cd " + conventions.DefaultWorkDir + " && " + commandLine
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func frameworkRequiresRunningEnvForFreshInstall(framework string) bool {
	return engine.FrameworkRequiresRunningEnvForFreshInstall(framework)
}

func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func needsRemoteEnvironment(opts BootstrapRuntimeOptions) bool {
	if opts.Fresh {
		return false
	}
	if opts.Plan {
		return true
	}
	// Remote is needed if we're cloning files, syncing media, or importing DB from remote.
	// We skip the requirement if --db-dump is provided and other remote-dependent flags are off.
	return opts.Clone || (opts.DBImport && opts.DBDump == "") || opts.MediaSync != ""
}

func shouldRunFrameworkPostClone(config engine.Config, opts BootstrapRuntimeOptions) bool {
	if !opts.ComposerInstall {
		return false
	}
	return engine.FrameworkSupportsPostClone(config.Framework)
}

func shouldIgnoreFrameworkPostCloneError(config engine.Config, err error, cwd string) bool {
	if err == nil {
		return false
	}
	errText := strings.ToLower(err.Error())

	// WordPress config might already exist or fail in non-critical ways
	if config.Framework == "wordpress" {
		return fileExists(filepath.Join(cwd, "wp-config.php"))
	}

	if config.Framework == "prestashop" {
		return fileExists(filepath.Join(cwd, "app", "config", "parameters.php"))
	}

	if !strings.Contains(errText, "composer install failed") {
		return false
	}
	return fileExists(filepath.Join(cwd, "vendor", "autoload.php"))
}

func shouldSkipBootstrapMediaSync(config engine.Config, opts BootstrapRuntimeOptions) (bool, string) {
	if opts.MediaSync == "" {
		return true, "media sync is disabled"
	}
	if opts.Clone && opts.CodeOnly {
		return true, "code-only mode"
	}

	remoteCfg, ok := config.Remotes[opts.Source]
	if !ok {
		return false, ""
	}

	_, remoteMediaPath := engine.ResolveRemotePaths(config, opts.Source)
	remoteMediaPath = strings.TrimSpace(remoteMediaPath)
	if remoteMediaPath == "" {
		return true, "remote media path is empty"
	}

	if !bootstrapRemoteDirExists(opts.Source, remoteCfg, remoteMediaPath) {
		return true, fmt.Sprintf("remote media path does not exist: %s", remoteMediaPath)
	}

	return false, ""
}

func ShouldRunFrameworkPostCloneForTest(framework string, composerInstall bool) bool {
	return shouldRunFrameworkPostClone(engine.Config{Framework: framework}, BootstrapRuntimeOptions{ComposerInstall: composerInstall})
}

func ShouldIgnoreFrameworkPostCloneErrorForTest(framework string, err error, cwd string) bool {
	return shouldIgnoreFrameworkPostCloneError(engine.Config{Framework: framework}, err, cwd)
}
func ShouldSkipBootstrapMediaSyncForTest(config engine.Config, source string, mediaMode string, clone bool, codeOnly bool) (bool, string) {
	return shouldSkipBootstrapMediaSync(config, BootstrapRuntimeOptions{
		Source:    source,
		MediaSync: mediaMode,
		Clone:     clone,
		CodeOnly:  codeOnly,
	})
}

func SetGovardSubcommandRunnerForTest(fn func(cmd *cobra.Command, args ...string) error) func() {
	previous := govardSubcommandRunner
	govardSubcommandRunner = fn
	return func() {
		govardSubcommandRunner = previous
	}
}

func SetPHPContainerShellRunnerForTest(fn func(config engine.Config, commandLine string) error) func() {
	previous := phpContainerShellRunner
	if fn != nil {
		phpContainerShellRunner = fn
	}
	return func() {
		phpContainerShellRunner = previous
	}
}

func BuildPHPContainerShellCommandForTest(commandLine string) string {
	return buildPHPContainerShellCommand(commandLine)
}

func init() {
	bootstrapCmd.Flags().SortFlags = false

	// 1. Clone Mode
	bootstrapCmd.Flags().BoolVarP(&bootstrapClone, "clone", "c", false, "Rsync source files from remote before composer/DB/media steps (use when you have no local git checkout)")
	bootstrapCmd.Flags().BoolVar(&bootstrapCodeOnly, "code-only", false, "Clone code only (skip DB/media)")

	// 2. Fresh Mode
	bootstrapCmd.Flags().BoolVar(&bootstrapFresh, "fresh", false, "Create a fresh project install")
	bootstrapCmd.Flags().StringVar(&bootstrapFramework, "framework", "", "Framework to use when init is required")
	bootstrapCmd.Flags().StringVar(&bootstrapFrameworkVersion, "framework-version", "", "Framework version (e.g. 2.4.7 for Magento, 11 for Laravel)")
	bootstrapCmd.Flags().StringVarP(&bootstrapMetaPackage, "meta-package", "p", defaultBootstrapMetaPackage, "Composer meta-package for fresh install (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapMageUsername, "mage-username", "", "Magento repo username for auth.json bootstrap (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapMagePassword, "mage-password", "", "Magento repo password for auth.json bootstrap (Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapIncludeSample, "include-sample", false, "Install sample data (fresh install, Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapHyvaInstall, "hyva-install", false, "Install Hyva default theme (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapHyvaToken, "hyva-token", defaultBootstrapHyvaToken, "Hyva repository token (Magento only)")

	// 3. Source Selection
	bootstrapCmd.Flags().StringVarP(&bootstrapEnv, "environment", "e", "", "Source environment (default: auto-select staging or dev)")
	bootstrapCmd.Flags().StringVar(&bootstrapEnv, "remote", "", "Alias for --environment")
	bootstrapCmd.Flags().StringVar(&bootstrapDBDump, "db-dump", "", "Import database from a local dump file")

	// 4. Scopes & Framework Special
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipComposer, "no-composer", false, "Skip composer install")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipDB, "no-db", false, "Skip database import")
	bootstrapCmd.Flags().BoolVar(&bootstrapNoStreamDB, "no-stream-db", false, "Disable stream-db import mode")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipMedia, "no-media", false, "Skip media sync")
	bootstrapCmd.Flags().StringVar(&bootstrapMediaModeFlag, "media", "", "Media sync mode: optimized (default when used without a value), all, catalog (Magento only), minimal, or none")
	if mediaFlag := bootstrapCmd.Flags().Lookup("media"); mediaFlag != nil {
		mediaFlag.NoOptDefVal = MediaSyncOptimized
	}
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipAdmin, "no-admin", false, "Skip admin user creation (Magento only)")

	// 5. Privacy & Data Filtering
	bootstrapCmd.Flags().BoolVarP(&bootstrapNoNoise, "no-noise", "N", false, "Exclude ephemeral/noise tables and directories from sync (logs, caches, etc)")
	bootstrapCmd.Flags().BoolVarP(&bootstrapNoPII, "no-pii", "S", false, "Exclude PII/sensitive tables from database sync (users, orders, passwords, etc)")

	// 6. Transfer & Sync Options
	bootstrapCmd.Flags().BoolVar(&bootstrapDelete, "delete", false, "Delete files on destination that are missing on source (media/files sync)")
	bootstrapCmd.Flags().BoolVarP(&bootstrapAssumeYes, "yes", "y", false, "Skip confirmation and proceed with bootstrap")
	bootstrapCmd.Flags().BoolVar(&bootstrapNoCompress, "no-compress", false, "Disable rsync compression during transfer")
	bootstrapCmd.Flags().StringArrayVarP(&bootstrapExclude, "exclude", "X", nil, "Exclude patterns for file/media sync")

	// 7. UX & Execution Control
	bootstrapCmd.Flags().BoolVar(&bootstrapFixDeps, "fix-deps", false, "Run project custom fix-deps command before bootstrap")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipUp, "no-up", false, "Skip starting local containers before bootstrap steps")
	bootstrapCmd.Flags().BoolVar(&bootstrapPlan, "plan", false, "Print the bootstrap plan and exit")
}

func NeedsRemoteEnvironmentForTest(opts BootstrapRuntimeOptions) bool {
	return needsRemoteEnvironment(opts)
}
