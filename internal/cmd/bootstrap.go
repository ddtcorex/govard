package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	bootstrapIncludeProduct   bool
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [flags]",
	Short: "Bootstrap local environment: import DB/media from remote, or full clone with --clone",
	Long: `Quickly set up a local project from a remote environment or a fresh installation.
Ideal for onboarding new team members or re-initialising a local workspace.

Two primary modes:
  Default (no --clone): Starts the local environment, runs composer install, imports the
    remote database, and syncs media — but does NOT rsync the source code files. Use this
    when your source code is already checked out from Git.
  --clone: Performs a full file rsync FROM the remote before the steps above. Use this only
    when you need an exact copy of the remote source (e.g. first-time onboarding without Git).

Framework Specifics:
- Magento 2: Automates auth.json, env.php, database import, media sync, and admin user creation.
- Symfony/Laravel: Handles .env generation and composer install.
- WordPress: Configures wp-config.php and imports database.

Case Studies:
- Day-to-day refresh: 'govard bootstrap -e staging' — syncs DB and media, keeps your local git tree.
- First-time onboarding (no git clone): 'govard bootstrap --clone -e staging' — pulls all source files.
- Fresh Start: 'govard bootstrap --framework magento2 --fresh --framework-version 2.4.7' — clean Magento install from Composer.
- Code only: 'govard bootstrap --clone --code-only -e dev' — files only, skip DB and media.
- Specify Framework: 'govard bootstrap --framework magento2' — Ensures Magento 2 environment if initialization is required.`,
	Example: `  # Refresh DB + media from dev (default — does NOT overwrite source files)
  govard bootstrap --environment dev

  # Full clone (source files + DB + media) from staging
  govard bootstrap --clone --environment staging

  # Fresh Magento 2.4.7 install with sample data
  govard bootstrap --framework magento2 --fresh --framework-version 2.4.7 --include-sample

  # Clone from dev but skip media sync
  govard bootstrap --clone --environment dev --no-media

  # Clone source code only (skip DB and media)
  govard bootstrap --clone --code-only --environment dev

  # Bootstrap and ensure Magento 2 if init runs
  govard bootstrap --framework magento2`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		pterm.DefaultHeader.Println("Govard Bootstrap")
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

		opts, err := resolveBootstrapOptions(cmd)
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
		supportedFrameworks := []string{"magento2", "laravel", "symfony"}
		if opts.Fresh {
			supportedFrameworks = []string{"magento2", "laravel", "symfony", "openmage", "drupal", "wordpress", "nextjs", "shopware", "cakephp"}
		}

		if !stringSliceContains(supportedFrameworks, config.Framework) {
			return fmt.Errorf("bootstrap currently supports these project types: %s (detected: %s)",
				strings.Join(supportedFrameworks, ", "), config.Framework)
		}

		maybeAutoDetectBootstrapVersion(config, &opts)

		if opts.FixDeps {
			runBootstrapFixDeps(cmd, opts)
		}

		if !opts.SkipUp {
			if err := runGovardSubcommand(cmd, "env", "up"); err != nil {
				return fmt.Errorf("failed to start local environment: %w", err)
			}
		}

		if !opts.Fresh {
			if err := runBootstrapRemote(cmd, config, opts); err != nil {
				return err
			}
		} else {
			if err := runBootstrapFrameworkFreshInstall(cmd, config, opts); err != nil {
				return err
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
	configPath := filepath.Join(cwd, engine.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check %s: %w", engine.BaseConfigFile, err)
	}

	pterm.Info.Printf("%s not found. Running `govard init` first.\n", engine.BaseConfigFile)
	initArgs := []string{"init"}
	if bootstrapFramework != "" {
		initArgs = append(initArgs, "--framework", bootstrapFramework)
	}
	if bootstrapFrameworkVersion != "" {
		initArgs = append(initArgs, "--framework-version", bootstrapFrameworkVersion)
	}
	return runGovardSubcommand(cmd, initArgs...)
}

var govardSubcommandRunner = func(cmd *cobra.Command, args ...string) error {
	executablePath, err := os.Executable()
	commandPath := "govard"
	if err == nil && strings.TrimSpace(executablePath) != "" {
		commandPath = executablePath
	}

	command := exec.Command(commandPath, args...)
	command.Dir, _ = os.Getwd()
	command.Stdin = os.Stdin
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = cmd.ErrOrStderr()
	return command.Run()
}

func runGovardSubcommand(cmd *cobra.Command, args ...string) error {
	return govardSubcommandRunner(cmd, args...)
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
	containerName := fmt.Sprintf("%s-php-1", config.ProjectName)
	dockerArgs := []string{"exec"}
	if stdinIsTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	}

	cwd, _ := os.Getwd()
	authPath := filepath.Join(cwd, "auth.json")
	if data, err := os.ReadFile(authPath); err == nil {
		dockerArgs = append(dockerArgs, "-e", "COMPOSER_AUTH="+string(data))
	}

	if user := resolveProjectExecUser(config, "www-data"); strings.TrimSpace(user) != "" {
		dockerArgs = append(dockerArgs, "-u", user)
	}
	dockerArgs = append(dockerArgs, "-w", "/var/www/html", containerName, "sh", "-lc", commandLine)
	dockerCmd := exec.Command("docker", dockerArgs...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	return dockerCmd.Run()
}

func shellQuote(raw string) string {
	if raw == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(raw, "'", `'"'"'`) + "'"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func shouldRunSymfonyPostClone(config engine.Config, opts bootstrapRuntimeOptions) bool {
	return config.Framework == "symfony" && opts.ComposerInstall
}

func shouldIgnoreSymfonyPostCloneError(err error, cwd string) bool {
	if err == nil {
		return false
	}
	if !strings.Contains(strings.ToLower(err.Error()), "composer install failed") {
		return false
	}
	return fileExists(filepath.Join(cwd, "vendor", "autoload.php"))
}

func shouldSkipBootstrapMediaSync(config engine.Config, opts bootstrapRuntimeOptions) (bool, string) {
	if !opts.MediaSync {
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

func ShouldRunSymfonyPostCloneForTest(framework string, composerInstall bool) bool {
	return shouldRunSymfonyPostClone(engine.Config{Framework: framework}, bootstrapRuntimeOptions{ComposerInstall: composerInstall})
}

func ShouldIgnoreSymfonyPostCloneErrorForTest(err error, cwd string) bool {
	return shouldIgnoreSymfonyPostCloneError(err, cwd)
}

func ShouldSkipBootstrapMediaSyncForTest(config engine.Config, source string, mediaSync bool, clone bool, codeOnly bool) (bool, string) {
	return shouldSkipBootstrapMediaSync(config, bootstrapRuntimeOptions{
		Source:    source,
		MediaSync: mediaSync,
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

func init() {
	bootstrapCmd.Flags().BoolVarP(&bootstrapClone, "clone", "c", false, "Rsync source files from remote before composer/DB/media steps (use when you have no local git checkout)")
	bootstrapCmd.Flags().BoolVar(&bootstrapCodeOnly, "code-only", false, "Clone code only (skip DB/media)")
	bootstrapCmd.Flags().BoolVar(&bootstrapFresh, "fresh", false, "Create a fresh project install")
	bootstrapCmd.Flags().BoolVar(&bootstrapIncludeSample, "include-sample", false, "Install sample data (fresh install, Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipDB, "no-db", false, "Skip database import")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipMedia, "no-media", false, "Skip media sync")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipComposer, "no-composer", false, "Skip composer install")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipAdmin, "no-admin", false, "Skip admin user creation (Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapNoStreamDB, "no-stream-db", false, "Disable stream-db import mode")
	bootstrapCmd.Flags().StringVarP(&bootstrapEnv, "environment", "e", "dev", "Source environment")
	bootstrapCmd.Flags().StringVar(&bootstrapEnv, "remote", "dev", "Alias for --environment")
	bootstrapCmd.Flags().StringVar(&bootstrapFramework, "framework", "", "Framework to use when init is required")
	bootstrapCmd.Flags().StringVar(&bootstrapFrameworkVersion, "framework-version", "", "Framework version (e.g. 2.4.7 for Magento, 11 for Laravel)")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipUp, "skip-up", false, "Skip starting local containers before bootstrap steps")
	bootstrapCmd.Flags().StringVarP(&bootstrapMetaPackage, "meta-package", "p", defaultBootstrapMetaPackage, "Composer meta-package for fresh install (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapDBDump, "db-dump", "", "Import database from a local dump file")
	bootstrapCmd.Flags().BoolVar(&bootstrapFixDeps, "fix-deps", false, "Run project custom fix-deps command before bootstrap")
	bootstrapCmd.Flags().BoolVar(&bootstrapHyvaInstall, "hyva-install", false, "Install Hyva default theme (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapHyvaToken, "hyva-token", defaultBootstrapHyvaToken, "Hyva repository token (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapMageUsername, "mage-username", "", "Magento repo username for auth.json bootstrap (Magento only)")
	bootstrapCmd.Flags().StringVar(&bootstrapMagePassword, "mage-password", "", "Magento repo password for auth.json bootstrap (Magento only)")
	bootstrapCmd.Flags().BoolVarP(&bootstrapAssumeYes, "yes", "y", false, "Assume yes for non-critical bootstrap prompts")
	bootstrapCmd.Flags().BoolVar(&bootstrapIncludeProduct, "include-product", false, "Include catalog product images during media sync (Magento only)")
}
