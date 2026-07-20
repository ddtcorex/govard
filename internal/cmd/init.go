package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init [flags]",
	Short: "Initialize a new project configuration",
	Long: `Initialize a Govard project configuration in the current directory.
It automatically detects the framework (Magento, Laravel, Symfony, etc.) and generates a .govard.yml file.
If detection fails, it prompts you to select a framework (defaulting to 'custom').

Common Framework Versions:
- Magento: 2.4.4, 2.4.5, 2.4.6, 2.4.7
- Laravel: 10, 11
- Symfony: 6.4, 7.0
- Shopware: 6.5, 6.6

Case Studies:
- New Project: Run 'govard init' in an empty folder to start a new app from scratch.
- Existing Project: Run 'govard init' to containerize an existing codebase.
- Migrate from DDEV: Use --migrate-from ddev to import settings from an existing DDEV setup.`,
	Example: `  # Auto-detect framework and initialize
  govard init

  # Explicitly set the framework
  govard init --framework magento2

  # Initialize from a DDEV configuration
  govard init --migrate-from ddev

  # Specify framework version during init
  govard init --framework laravel --framework-version 11`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Initialization ")
		fmt.Println()
		startedAt := time.Now()

		fmt.Println("🔍 Detecting project framework...")
		if initAssumeYes {
			_ = os.Setenv("GOVARD_ASSUME_YES", "true")
		}
		cwd, _ := os.Getwd()
		configForObservability := engine.Config{}
		defer func() {
			status := engine.OperationStatusSuccess
			message := "init completed"
			if err != nil {
				status = engine.OperationStatusFailure
				message = err.Error()
			}
			writeOperationEventBestEffort(
				"init.run",
				status,
				configForObservability,
				"",
				"",
				message,
				"",
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(configForObservability, cwd, "init")
			}
		}()
		existingConfig, hasExistingConfig := loadExistingInitConfig(cwd)
		var migrated engine.MigrationResult
		if migrateFrom != "" {
			var migrateErr error
			handled := false
			switch strings.ToLower(migrateFrom) {
			case "ddev":
				migrated, migrateErr = engine.MigrateFromDDEV(cwd)
				handled = true
			case "warden":
				migrated, migrateErr = engine.MigrateFromWarden(cwd)
				handled = true
			default:
				pterm.Warning.Printf("Unknown migration source: %s. Skipping migration.\n", migrateFrom)
			}
			if migrateErr != nil {
				pterm.Warning.Printf("Migration from %s failed: %v\n", migrateFrom, migrateErr)
			} else if handled {
				pterm.Success.Printf("Migrated configuration from %s\n", migrateFrom)
			}
		}

		metadata := engine.DetectFramework(cwd)
		if migrated.Framework != "" {
			metadata.Framework = migrated.Framework
		}
		if initFramework != "" {
			metadata.Framework = strings.ToLower(initFramework)
			if metadata.Framework == "magento" {
				metadata.Framework = "magento2"
			}
		}
		if initFrameworkVersion != "" {
			metadata.Version = initFrameworkVersion
		}
		if metadata.Framework == "" || metadata.Framework == "generic" {
			frameworkMap := map[string]string{
				"CakePHP":    "cakephp",
				"Custom":     "custom",
				"Drupal":     "drupal",
				"Emdash":     "emdash",
				"Laravel":    "laravel",
				"Magento 1":  "magento1",
				"Magento 2":  "magento2",
				"Next.js":    "nextjs",
				"OpenMage":   "openmage",
				"PrestaShop": "prestashop",
				"Shopware":   "shopware",
				"Symfony":    "symfony",
				"WordPress":  "wordpress",
			}

			frameworkDisplayOptions := make([]string, 0, len(frameworkMap))
			for k := range frameworkMap {
				frameworkDisplayOptions = append(frameworkDisplayOptions, k)
			}
			sort.Strings(frameworkDisplayOptions)

			selectedDisplay := selectOption("Select project framework", frameworkDisplayOptions, "Custom")
			metadata.Framework = frameworkMap[selectedDisplay]

		}

		if metadata.Version != "" {
			pterm.Success.Printf("Detected Framework: %s (%s)\n", cases.Title(language.Und).String(metadata.Framework), metadata.Version)
		} else {
			pterm.Success.Printf("Detected Framework: %s\n", cases.Title(language.Und).String(metadata.Framework))
		}

		profileResult, err := engine.ResolveRuntimeProfile(metadata.Framework, metadata.Version)
		if err != nil {
			return fmt.Errorf("resolve runtime profile: %w", err)
		}

		webServer := profileResult.Profile.WebServer
		search := profileResult.Profile.Search
		cache := profileResult.Profile.Cache
		queue := profileResult.Profile.Queue
		dbType := profileResult.Profile.DB
		dbVersion := profileResult.Profile.DBVersion
		phpVersion := profileResult.Profile.PHPVersion
		nodeVersion := profileResult.Profile.NodeVersion
		composerVersion := profileResult.Profile.ComposerVersion
		xdebugSession := profileResult.Profile.XdebugSession
		webRoot := profileResult.Profile.WebRoot
		enableVarnish := metadata.Framework == "magento2" && migrateFrom == "" && !hasExistingConfig

		if metadata.Framework == "custom" {
			pterm.Info.Println("Customize your stack services for the custom framework.")
			if webRoot == "" {
				webRoot = "/public"
			}
			webServer = selectOption("Web server", []string{"nginx", "apache", "hybrid"}, webServer)
			cache = selectOption("Cache service", []string{"none", "redis", "valkey"}, cache)
			search = selectOption("Search service", []string{"none", "opensearch", "elasticsearch"}, search)
			queue = selectOption("Queue service", []string{"none", "rabbitmq"}, queue)

			dbType = selectOption("Database", []string{"mariadb", "mysql", "none"}, dbType)
			switch dbType {
			case "mysql":
				dbVersion = textInput("MySQL version", "8.4")
			case "mariadb":
				dbVersion = textInput("MariaDB version", dbVersion)
			default:
				dbVersion = ""
			}

			phpVersion = selectOption("PHP version", []string{"none", "8.4", "8.3", "8.2", "8.1"}, phpVersion)
			if phpVersion != "none" {
				composerVersion = textInput("Composer version", composerVersion)
			} else {
				composerVersion = ""
			}
			nodeVersion = textInput("Node.js version", nodeVersion)
			webRoot = textInput("Web root (e.g. /public, leave empty for project root)", webRoot)
			xdebugSession = textInput("Xdebug session cookie value", xdebugSession)
			if initAssumeYes {
				enableVarnish = false
			} else {
				enableVarnish, _ = pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show("Enable Varnish?")
			}
		}

		config := engine.Config{
			ProjectName:      engine.NormalizeProjectName(filepath.Base(cwd)),
			Framework:        metadata.Framework,
			FrameworkVersion: metadata.Version,
			Domain:           engine.InferProjectDomain(cwd),
			Stack: engine.Stack{
				PHPVersion:      phpVersion,
				NodeVersion:     nodeVersion,
				ComposerVersion: composerVersion,
				DBVersion:       dbVersion,
				WebRoot:         webRoot,
				XdebugSession:   xdebugSession,
				Services: engine.Services{
					WebServer: webServer,
					DB:        dbType,
					Search:    search,
					Cache:     cache,
					Queue:     queue,
				},
				Features: engine.Features{
					Xdebug:  true,
					Varnish: enableVarnish,
					Cache:   cache != "" && cache != "none",
					Search:  search != "" && search != "none",
					Queue:   queue != "" && queue != "none",
				},
			},
		}

		if migrated.ProjectName != "" {
			config.ProjectName = engine.NormalizeProjectName(migrated.ProjectName)
			if strings.HasSuffix(strings.ToLower(strings.TrimSpace(migrated.ProjectName)), ".test") {
				config.Domain = strings.ToLower(strings.TrimSpace(migrated.ProjectName))
			} else {
				config.Domain = fmt.Sprintf("%s.test", config.ProjectName)
			}
		}
		if migrated.PHPVersion != "" {
			config.Stack.PHPVersion = migrated.PHPVersion
		}
		if migrated.NodeVersion != "" {
			config.Stack.NodeVersion = migrated.NodeVersion
		}
		if migrated.ComposerVersion != "" {
			config.Stack.ComposerVersion = migrated.ComposerVersion
		}
		if migrated.DB != "" {
			config.Stack.Services.DB = migrated.DB
		}
		if migrated.DBVersion != "" {
			config.Stack.DBVersion = migrated.DBVersion
		}
		if migrated.WebRoot != "" {
			config.Stack.WebRoot = migrated.WebRoot
		}
		if migrated.TablePrefix != "" && engine.FrameworkSupportsTablePrefix(config.Framework) {
			config.TablePrefix = migrated.TablePrefix
		}
		if migrateFrom != "" {
			// Search
			if migrated.SearchService != "" {
				config.Stack.Services.Search = migrated.SearchService
				config.Stack.SearchVersion = migrated.SearchVersion
				config.Stack.Features.Search = migrated.SearchService != "none"
			} else {
				config.Stack.Services.Search = "none"
				config.Stack.Features.Search = false
			}

			// Cache
			if migrated.CacheService != "" {
				config.Stack.Services.Cache = migrated.CacheService
				config.Stack.CacheVersion = migrated.CacheVersion
				config.Stack.Features.Cache = migrated.CacheService != "none"
			} else {
				config.Stack.Services.Cache = "none"
				config.Stack.Features.Cache = false
			}

			// Queue
			if migrated.QueueService != "" {
				config.Stack.Services.Queue = migrated.QueueService
				config.Stack.QueueVersion = migrated.QueueVersion
				config.Stack.Features.Queue = migrated.QueueService != "none"
			} else {
				config.Stack.Services.Queue = "none"
				config.Stack.Features.Queue = false
			}

			// Varnish
			config.Stack.Features.Varnish = migrated.VarnishEnabled
			if migrated.VarnishVersion != "" {
				config.Stack.VarnishVersion = migrated.VarnishVersion
			}
		}

		if len(migrated.Remotes) > 0 {
			config.Remotes = migrated.Remotes
		}

		if config.Stack.Services.DB == "none" {
			config.Stack.DBVersion = ""
		}
		if hasExistingConfig {
			if existingConfig.ProjectName != "" {
				config.ProjectName = existingConfig.ProjectName
				config.Domain = existingConfig.Domain
			}
			if existingConfig.TablePrefix != "" {
				config.TablePrefix = existingConfig.TablePrefix
			}
			if existingConfig.Stack.PHPVersion != "" {
				config.Stack.PHPVersion = existingConfig.Stack.PHPVersion
			}
			if existingConfig.Stack.NodeVersion != "" {
				config.Stack.NodeVersion = existingConfig.Stack.NodeVersion
			}
			if existingConfig.Stack.ComposerVersion != "" {
				config.Stack.ComposerVersion = existingConfig.Stack.ComposerVersion
			}
			if existingConfig.Stack.Services.DB != "" {
				config.Stack.Services.DB = existingConfig.Stack.Services.DB
			}
			if existingConfig.Stack.DBVersion != "" {
				config.Stack.DBVersion = existingConfig.Stack.DBVersion
			}
			if existingConfig.Stack.WebRoot != "" {
				config.Stack.WebRoot = existingConfig.Stack.WebRoot
			}
			if existingConfig.Stack.Services.WebServer != "" {
				config.Stack.Services.WebServer = existingConfig.Stack.Services.WebServer
			}
			if existingConfig.Stack.Services.Search != "" {
				config.Stack.Services.Search = existingConfig.Stack.Services.Search
				config.Stack.SearchVersion = existingConfig.Stack.SearchVersion
				config.Stack.Features.Search = existingConfig.Stack.Services.Search != "none"
			}
			if existingConfig.Stack.Services.Cache != "" {
				config.Stack.Services.Cache = existingConfig.Stack.Services.Cache
				config.Stack.CacheVersion = existingConfig.Stack.CacheVersion
				config.Stack.Features.Cache = existingConfig.Stack.Services.Cache != "none"
			}
			if existingConfig.Stack.Services.Queue != "" {
				config.Stack.Services.Queue = existingConfig.Stack.Services.Queue
				config.Stack.QueueVersion = existingConfig.Stack.QueueVersion
				config.Stack.Features.Queue = existingConfig.Stack.Services.Queue != "none"
			}
			if existingConfig.Stack.Features.Varnish {
				config.Stack.Features.Varnish = true
				config.Stack.VarnishVersion = existingConfig.Stack.VarnishVersion
			}

			if existingConfig.Remotes != nil {
				if config.Remotes == nil {
					config.Remotes = make(map[string]engine.RemoteConfig)
				}
				for k, v := range existingConfig.Remotes {
					config.Remotes[k] = v
				}
			}
			config.Hooks = existingConfig.Hooks
		}

		oldWebRoot := config.Stack.WebRoot

		configForObservability = config
		engine.NormalizeConfig(&config, cwd)

		if oldWebRoot == "/" && config.Stack.WebRoot != "/" && config.Stack.WebRoot != "" {
			pterm.Info.Printf("Corrected web_root from '/' to '%s' based on framework conventions\n", config.Stack.WebRoot)
		}
		if err := engine.ValidateProjectIdentityUniqueness(cwd, config); err != nil {
			return err
		}

		writableConfig := engine.PrepareConfigForWrite(config)

		data, err := yaml.Marshal(&writableConfig)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", conventions.BaseConfigFile, err)
		}
		if err := os.WriteFile(conventions.BaseConfigFile, data, conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("write %s: %w", conventions.BaseConfigFile, err)
		}
		pterm.Success.Println("✅ Generated .govard.yml")

		if err := engine.RenderBlueprint(cwd, config); err != nil {
			pterm.Warning.Printf("Failed to render compose file: %v\n", err)
			pterm.Info.Println("You can retry compose rendering later via `govard env up`.")
			return nil
		}
		composePath := engine.ComposeFilePathWithProfile(cwd, config.ProjectName, config.Profile)
		pterm.Success.Printf("✅ Rendered compose file at %s\n", composePath)

		if migrateFrom != "" {
			fmt.Println()
			var assumedSourceVolume string
			switch strings.ToLower(migrateFrom) {
			case "warden":
				assumedSourceVolume = migrated.ProjectName + "_dbdata"
			case "ddev":
				assumedSourceVolume = "ddev-" + migrated.ProjectName + "-db"
			}

			if assumedSourceVolume != "" && config.Stack.Services.DB != "none" && config.Stack.Services.DB != "" {
				if !initAssumeYes {
					proceed, err := pterm.DefaultInteractiveConfirm.
						WithDefaultValue(false).
						Show(fmt.Sprintf("Do you want to clone the existing database volume from %s ('%s') into Govard?", cases.Title(language.Und).String(migrateFrom), assumedSourceVolume))

					if err == nil && proceed {
						// Double-check if source volume exists
						checkCmd := exec.Command("docker", "volume", "inspect", assumedSourceVolume)
						if err := checkCmd.Run(); err == nil {
							cloneOpts := dbCommandOptions{
								Environment: "local",
								AssumeYes:   true, // Automatically accept sub-prompts since user opted-in
							}
							if errClone := runDBCloneVolume(cmd, config, cloneOpts, []string{assumedSourceVolume}); errClone != nil {
								pterm.Warning.Printf("Database clone failed: %v\n", errClone)
							}
						} else {
							pterm.Warning.Printf("Source volume '%s' not found. Skipping clone.\n", assumedSourceVolume)
						}
					}
				}
			}

			fmt.Println()
			pterm.Info.Printf("Running environment diagnostics to finalize migration from %s...\n", migrateFrom)

			// Auto-run doctor --fix but explicitly skip the interactive framework tune profile check
			// because the user already established their configuration during migration.
			opts := map[string]bool{"project.profile.sync": true}
			if err := ExecuteDoctor(cmd, false, true, false, "", opts); err != nil {
				pterm.Warning.Printf("Migration diagnostics encountered an issue: %v\n", err)
			}
		}

		return nil
	},
}

func loadExistingInitConfig(cwd string) (engine.Config, bool) {
	configPath := filepath.Join(cwd, conventions.BaseConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return engine.Config{}, false
	}
	var existing engine.Config
	if err := yaml.Unmarshal(data, &existing); err != nil {
		pterm.Warning.Printf("Could not load existing %s for merge: %v\n", conventions.BaseConfigFile, err)
		return engine.Config{}, false
	}
	return existing, true
}

var (
	initFramework        string
	initFrameworkVersion string
	migrateFrom          string
	initAssumeYes        bool
)

func init() {
	initCmd.Flags().StringVar(&initFramework, "framework", "", "Override detected framework (e.g., magento2)")
	initCmd.Flags().StringVar(&initFrameworkVersion, "framework-version", "", "Override detected framework version (e.g., 11)")
	initCmd.Flags().StringVar(&migrateFrom, "migrate-from", "", "Migrate configuration from another tool (ddev, warden)")
	initCmd.Flags().BoolVarP(&initAssumeYes, "yes", "y", false, "Assume yes to all prompts (non-interactive mode)")
}

func selectOption(title string, options []string, defaultOption string) string {
	if initAssumeYes {
		return defaultOption
	}
	printer := pterm.DefaultInteractiveSelect.WithOptions(options)
	if defaultOption != "" {
		printer = printer.WithDefaultOption(defaultOption)
	}
	result, err := printer.Show(title)
	if err != nil || result == "" {
		return defaultOption
	}
	return result
}

func textInput(title string, defaultValue string) string {
	if initAssumeYes {
		return defaultValue
	}
	printer := pterm.DefaultInteractiveTextInput.WithDefaultValue(defaultValue)
	result, err := printer.Show(title)
	if err != nil {
		return defaultValue
	}
	return result
}
