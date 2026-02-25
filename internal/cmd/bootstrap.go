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
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"

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
	bootstrapVersion          string
	bootstrapEnv              string
	bootstrapRecipe           string
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

type bootstrapRuntimeOptions struct {
	Source          string
	Clone           bool
	CodeOnly        bool
	Fresh           bool
	IncludeSample   bool
	DBImport        bool
	MediaSync       bool
	ComposerInstall bool
	AdminCreate     bool
	StreamDB        bool
	SkipUp          bool
	MetaPackage     string
	MetaVersion     string
	DBDump          string
	FixDeps         bool
	HyvaInstall     bool
	HyvaToken       string
	MageUsername    string
	MagePassword    string
	AssumeYes       bool
	IncludeProduct  bool
}

var bootstrapRemoteDirExists = func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool {
	probe := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, "test -d "+shellQuote(remotePath))
	return probe.Run() == nil
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [flags]",
	Short: "Bootstrap local project setup and clone a remote environment",
	Long: `Quickly set up a project by cloning from a remote environment or creating a fresh installation.
Ideal for onboarding new team members or starting a new project from scratch.

Framework Specifics:
- Magento 2: Automates auth.json, env.php, database import, media sync, and admin user creation.
- Symfony/Laravel: Handles .env generation and composer install.
- WordPress: Configures wp-config.php and imports database.

Case Studies:
- New Team Member: Run 'govard bootstrap --environment staging' to get a carbon copy of the staging site.
- Fresh Start: Run 'govard bootstrap --fresh --version 2.4.7' to start a clean Magento 2.4.7 project.
- Fast Sync: Use --code-only to skip DB and media if you only need the source code.`,
	Example: `  # Clone from staging environment (auto-detects framework)
  govard bootstrap --environment staging

  # Fresh Magento 2.4.7 install with sample data
  govard bootstrap --fresh --version 2.4.7 --include-sample

  # Clone from production but skip media files
  govard bootstrap --environment prod --no-media

  # Clone code only (skip DB and media)
  govard bootstrap --code-only`,
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

		config := loadFullConfig()
		configForObservability = config
		supportedRecipes := []string{"magento2", "magento1", "openmage", "symfony", "laravel", "drupal", "wordpress", "nextjs", "shopware", "cakephp"}
		if !stringSliceContains(supportedRecipes, config.Recipe) {
			return fmt.Errorf("bootstrap currently supports %s projects only (detected: %s)",
				strings.Join(supportedRecipes, ", "), config.Recipe)
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

		if opts.Clone {
			if err := runBootstrapClone(cmd, config, opts); err != nil {
				return err
			}
		}

		if opts.Fresh {
			if err := runBootstrapFrameworkFreshInstall(cmd, config, opts); err != nil {
				return err
			}
		}

		pterm.Success.Printf("Bootstrap completed in %s.\n", time.Since(startedAt).Round(time.Second))
		return nil
	},
}

func resolveBootstrapOptions(cmd *cobra.Command) (bootstrapRuntimeOptions, error) {
	opts := bootstrapRuntimeOptions{
		Source:          normalizeBootstrapSource(bootstrapEnv),
		Clone:           bootstrapClone,
		CodeOnly:        bootstrapCodeOnly,
		Fresh:           bootstrapFresh,
		IncludeSample:   bootstrapIncludeSample,
		DBImport:        !bootstrapSkipDB,
		MediaSync:       !bootstrapSkipMedia,
		ComposerInstall: !bootstrapSkipComposer,
		AdminCreate:     !bootstrapSkipAdmin,
		StreamDB:        !bootstrapNoStreamDB,
		SkipUp:          bootstrapSkipUp,
		MetaPackage:     strings.TrimSpace(bootstrapMetaPackage),
		MetaVersion:     strings.TrimSpace(bootstrapVersion),
		DBDump:          strings.TrimSpace(bootstrapDBDump),
		FixDeps:         bootstrapFixDeps,
		HyvaInstall:     bootstrapHyvaInstall,
		HyvaToken:       strings.TrimSpace(bootstrapHyvaToken),
		MageUsername:    strings.TrimSpace(bootstrapMageUsername),
		MagePassword:    strings.TrimSpace(bootstrapMagePassword),
		AssumeYes:       bootstrapAssumeYes,
		IncludeProduct:  bootstrapIncludeProduct,
	}

	if opts.MetaPackage == "" {
		opts.MetaPackage = defaultBootstrapMetaPackage
	}
	if opts.HyvaToken == "" {
		opts.HyvaToken = defaultBootstrapHyvaToken
	}
	cloneFlagExplicit := false
	if cmd != nil {
		cloneFlagExplicit = cmd.Flags().Changed("clone")
	}

	if opts.MetaVersion != "" {
		comparison, comparable := compareNumericDotVersions(opts.MetaVersion, "2.0.0")
		if !comparable || comparison < 0 {
			return bootstrapRuntimeOptions{}, fmt.Errorf("invalid --version value %q (must be Magento 2.0.0+)", opts.MetaVersion)
		}
	}
	if opts.Fresh && opts.Clone {
		if cloneFlagExplicit {
			return bootstrapRuntimeOptions{}, fmt.Errorf("--fresh and --clone cannot be used together")
		}
		opts.Clone = false
	}
	if opts.CodeOnly && !opts.Clone {
		return bootstrapRuntimeOptions{}, fmt.Errorf("--code-only requires --clone")
	}
	if opts.Fresh {
		opts.ComposerInstall = false
		opts.DBImport = false
		opts.MediaSync = false
	}
	if opts.Clone && opts.CodeOnly {
		opts.DBImport = false
		opts.MediaSync = false
	}

	return opts, nil
}

func normalizeBootstrapSource(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "dev"
	}
	return value
}

func runBootstrapClone(cmd *cobra.Command, config engine.Config, opts bootstrapRuntimeOptions) error {
	if _, ok := config.Remotes[opts.Source]; !ok {
		return fmt.Errorf("remote '%s' is not configured. Add it to remotes in %s", opts.Source, engine.BaseConfigFile)
	}

	if err := runGovardSubcommand(cmd, "remote", "test", opts.Source); err != nil {
		return fmt.Errorf("remote test failed for '%s': %w", opts.Source, err)
	}

	if err := runGovardSubcommand(cmd, bootstrapFileSyncArgs(opts)...); err != nil {
		return fmt.Errorf("file sync failed: %w", err)
	}

	cwd, _ := os.Getwd()

	if opts.ComposerInstall {
		if err := ensureBootstrapAuthJSON(config, opts); err != nil {
			return err
		}
		if err := runBootstrapComposerPrepare(config); err != nil {
			return err
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
	if err := bootstrapComposerDumpAutoload(cmd, cwd); err != nil {
		return err
	}

	if opts.DBImport {
		if err := runBootstrapDatabaseSync(cmd, opts); err != nil {
			return err
		}
	}

	if config.Recipe == "magento2" {
		if err := ensureBootstrapMagentoEnvPHP(config, opts); err != nil {
			return err
		}
	}

	if err := runGovardSubcommand(cmd, govardConfigureSubcommandArgs()...); err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	// Some Magento commands can invalidate generated classes that were previously indexed in classmaps.
	// Rebuild autoload once more so subsequent steps (admin user, smoke checks) do not fail on stale references.
	if err := bootstrapComposerDumpAutoload(cmd, cwd); err != nil {
		return err
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
	} else if config.Recipe == "symfony" {
		pterm.Info.Println("Skipping Symfony post-clone setup because composer install is disabled.")
	}

	if opts.AdminCreate && config.Recipe == "magento2" {
		runBootstrapAdminCreate(cmd, config)
	}

	if opts.MediaSync {
		if skip, reason := shouldSkipBootstrapMediaSync(config, opts); skip {
			pterm.Warning.Printf("Skipping media sync: %s\n", reason)
			pterm.Success.Printf("Bootstrap clone from '%s' completed.\n", opts.Source)
			return nil
		}
		args := []string{"sync", "--source", opts.Source, "--media"}
		if config.Recipe == "magento2" {
			args = append(args, bootstrapMagentoMediaSyncArgs(opts)...)
		}
		if err := runGovardSubcommand(cmd, args...); err != nil {
			return fmt.Errorf("media sync failed: %w", err)
		}
	}

	pterm.Success.Printf("Bootstrap clone from '%s' completed.\n", opts.Source)
	return nil
}

func bootstrapComposerDumpAutoload(cmd *cobra.Command, cwd string) error {
	if !fileExists(filepath.Join(cwd, "composer.json")) {
		return nil
	}
	if err := runGovardSubcommand(cmd, govardComposerSubcommandArgs("dump-autoload", "-o", "-n")...); err != nil {
		autoloadPath := filepath.Join(cwd, "vendor", "autoload.php")
		if !fileExists(autoloadPath) {
			return fmt.Errorf("composer autoload generation failed: %w", err)
		}
		pterm.Warning.Printf("composer dump-autoload skipped (%v).\n", err)
	}
	return nil
}

func runBootstrapComposerPrepare(config engine.Config) error {
	if err := runPHPContainerShellCommand(config, "rm -rf vendor"); err != nil {
		return fmt.Errorf("failed to clean vendor directory: %w", err)
	}
	return nil
}

func ensureBootstrapMagentoEnvPHP(config engine.Config, opts bootstrapRuntimeOptions) error {
	if config.Recipe != "magento2" {
		return nil
	}

	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, "app", "etc", "env.php")

	if info, err := os.Lstat(envPath); err == nil && (info.Mode()&os.ModeSymlink) != 0 {
		if _, err := os.Stat(envPath); err != nil {
			if err := os.Remove(envPath); err != nil {
				return fmt.Errorf("failed to remove env.php symlink: %w", err)
			}
		} else {
			return nil
		}
	} else if err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
		return fmt.Errorf("failed to create app/etc: %w", err)
	}

	cryptKey := "00000000000000000000000000000000"
	if remoteCfg, ok := config.Remotes[opts.Source]; ok {
		if metadata, err := remote.ProbeMagento2Environment(opts.Source, remoteCfg); err == nil {
			if strings.TrimSpace(metadata.CryptKey) != "" {
				cryptKey = strings.TrimSpace(metadata.CryptKey)
			}
		} else {
			pterm.Warning.Printf("Could not extract crypt/key from remote env.php (%v). Using fallback key.\n", err)
		}
	}

	containerName := fmt.Sprintf("%s-db-1", config.ProjectName)
	localDB := resolveLocalDBCredentials(containerName)

	template := fmt.Sprintf(`<?php
return [
    'backend' => [
        'frontName' => 'admin'
    ],
    'crypt' => [
        'key' => %q
    ],
    'db' => [
        'table_prefix' => '',
        'connection' => [
            'default' => [
                'host' => 'db',
                'dbname' => %q,
                'username' => %q,
                'password' => %q,
                'active' => '1'
            ],
            'indexer' => [
                'host' => 'db',
                'dbname' => %q,
                'username' => %q,
                'password' => %q,
                'active' => '1'
            ]
        ]
    ],
    'resource' => [
        'default_setup' => [
            'connection' => 'default'
        ]
    ],
    'x-frame-options' => 'SAMEORIGIN',
    'MAGE_MODE' => 'developer',
    'session' => [
        'save' => 'files'
    ]
];
`, cryptKey,
		localDB.Database, localDB.Username, localDB.Password,
		localDB.Database, localDB.Username, localDB.Password,
	)

	if err := os.WriteFile(envPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write app/etc/env.php: %w", err)
	}

	pterm.Info.Println("Generated local app/etc/env.php for bootstrap.")
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

func ensureBootstrapInit(cmd *cobra.Command, cwd string) error {
	configPath := filepath.Join(cwd, engine.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check %s: %w", engine.BaseConfigFile, err)
	}

	pterm.Info.Printf("%s not found. Running `govard init` first.\n", engine.BaseConfigFile)
	initArgs := []string{"init"}
	if bootstrapRecipe != "" {
		initArgs = append(initArgs, "--recipe", bootstrapRecipe)
	}
	if bootstrapFrameworkVersion != "" {
		initArgs = append(initArgs, "--framework-version", bootstrapFrameworkVersion)
	}
	return runGovardSubcommand(cmd, initArgs...)
}

func ensureBootstrapAuthJSON(config engine.Config, opts bootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()
	authPath := filepath.Join(cwd, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		ensureAuthInGitignore(cwd)
		return nil
	}

	globalAuthPath := filepath.Join(os.Getenv("HOME"), ".composer", "auth.json")
	if _, err := os.Stat(globalAuthPath); err == nil {
		if opts.AssumeYes || shouldUseGlobalAuthByDefault() {
			data, readErr := os.ReadFile(globalAuthPath)
			if readErr != nil {
				return fmt.Errorf("failed reading global auth.json: %w", readErr)
			}
			if writeErr := os.WriteFile(authPath, data, 0600); writeErr != nil {
				return fmt.Errorf("failed writing project auth.json: %w", writeErr)
			}
			pterm.Success.Printf("Copied global auth.json from %s\n", globalAuthPath)
			ensureAuthInGitignore(cwd)
			return nil
		}
	}

	if opts.MageUsername != "" && opts.MagePassword != "" {
		payload := fmt.Sprintf("{\n    \"http-basic\": {\n        \"repo.magento.com\": {\n            \"username\": %q,\n            \"password\": %q\n        }\n    }\n}\n", opts.MageUsername, opts.MagePassword)
		if err := os.WriteFile(authPath, []byte(payload), 0600); err != nil {
			return fmt.Errorf("failed writing auth.json from CLI credentials: %w", err)
		}
		ensureAuthInGitignore(cwd)
		return nil
	}

	pterm.Warning.Println("auth.json not found. Provide --mage-username/--mage-password or create auth.json before composer-related steps.")
	return nil
}

func ensureAuthInGitignore(cwd string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "auth.json") {
		return
	}
	lines := content
	if !strings.HasSuffix(lines, "\n") {
		lines += "\n"
	}
	lines += "/auth.json\n"
	_ = os.WriteFile(gitignorePath, []byte(lines), 0644)
}

func shouldUseGlobalAuthByDefault() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (stdinInfo.Mode() & os.ModeCharDevice) == 0
}

func runGovardSubcommand(cmd *cobra.Command, args ...string) error {
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
	return config.Recipe == "symfony" && opts.ComposerInstall
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

func ShouldRunSymfonyPostCloneForTest(recipe string, composerInstall bool) bool {
	return shouldRunSymfonyPostClone(engine.Config{Recipe: recipe}, bootstrapRuntimeOptions{ComposerInstall: composerInstall})
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

func SetBootstrapRemoteDirExistsForTest(fn func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool) func() {
	previous := bootstrapRemoteDirExists
	bootstrapRemoteDirExists = fn
	return func() {
		bootstrapRemoteDirExists = previous
	}
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

func bootstrapMagentoMediaSyncArgs(opts bootstrapRuntimeOptions) []string {
	excludes := []string{
		"*.gz",
		"*.zip",
		"*.tar",
		"*.7z",
		"*.sql",
		"tmp",
		"itm",
		"import",
		"export",
		"importexport",
		"captcha",
		"analytics",
		"catalog/product.rm",
		"catalog/product/product",
		"opti_image",
		"webp_image",
		"webp_cache",
		"shoppingfeed",
		"amasty/blog/cache",
	}

	// Keep product images excluded by default to make media sync faster.
	// When explicitly requested, still exclude the cache folder.
	if opts.IncludeProduct {
		excludes = append(excludes, "catalog/product/cache")
	} else {
		excludes = append(excludes, "catalog/product")
	}

	args := make([]string, 0, len(excludes)*2)
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	return args
}

func init() {
	bootstrapCmd.Flags().BoolVarP(&bootstrapClone, "clone", "c", true, "Clone project from remote")
	bootstrapCmd.Flags().BoolVar(&bootstrapCodeOnly, "code-only", false, "Clone code only (skip DB/media)")
	bootstrapCmd.Flags().BoolVar(&bootstrapFresh, "fresh", false, "Create a fresh project install")
	bootstrapCmd.Flags().BoolVar(&bootstrapIncludeSample, "include-sample", false, "Install sample data (fresh install, Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipDB, "no-db", false, "Skip database import")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipMedia, "no-media", false, "Skip media sync")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipComposer, "no-composer", false, "Skip composer install")
	bootstrapCmd.Flags().BoolVar(&bootstrapSkipAdmin, "no-admin", false, "Skip admin user creation (Magento only)")
	bootstrapCmd.Flags().BoolVar(&bootstrapNoStreamDB, "no-stream-db", false, "Disable stream-db import mode")
	bootstrapCmd.Flags().StringVar(&bootstrapVersion, "version", "", "Framework version for fresh install (e.g. 2.4.7 for Magento, 11 for Laravel)")
	bootstrapCmd.Flags().StringVarP(&bootstrapEnv, "environment", "e", "dev", "Source environment")
	bootstrapCmd.Flags().StringVar(&bootstrapEnv, "remote", "dev", "Alias for --environment")
	bootstrapCmd.Flags().StringVarP(&bootstrapRecipe, "recipe", "r", "", "Recipe to use when init is required")
	bootstrapCmd.Flags().StringVar(&bootstrapFrameworkVersion, "framework-version", "", "Framework version when init is required")
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
