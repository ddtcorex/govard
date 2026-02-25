package cmd

import (
	"fmt"
	"govard/internal/engine"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init [flags]",
	Short: "Initialize a new project configuration",
	Long: `Initialize a Govard project configuration in the current directory.
It automatically detects the framework (Magento, Laravel, Symfony, etc.) and generates a .govard.yml file.
If detection fails, it falls back to a 'custom' recipe with interactive prompts.

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

  # Explicitly set the framework/recipe
  govard init --recipe magento2

  # Initialize from a DDEV configuration
  govard init --migrate-from ddev

  # Specify framework version during init
  govard init --recipe laravel --framework-version 11`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		pterm.DefaultHeader.Println("Govard Initialization")
		startedAt := time.Now()

		fmt.Println("🔍 Detecting project framework...")
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
		if migrated.Recipe != "" {
			metadata.Framework = migrated.Recipe
		}
		if initRecipe != "" {
			metadata.Framework = strings.ToLower(initRecipe)
		}
		if initFrameworkVersion != "" {
			metadata.Version = initFrameworkVersion
		}
		if metadata.Framework == "" || metadata.Framework == "generic" {
			pterm.Warning.Println("Could not detect framework confidently. Falling back to custom recipe defaults.")
			metadata.Framework = "custom"
			metadata.Version = ""
		}

		if metadata.Version != "" {
			pterm.Success.Printf("Detected Framework: %s (%s)\n", strings.Title(metadata.Framework), metadata.Version)
		} else {
			pterm.Success.Printf("Detected Framework: %s\n", strings.Title(metadata.Framework))
		}

		profileResult, err := engine.ResolveRuntimeProfile(metadata.Framework, metadata.Version)
		if err != nil {
			return fmt.Errorf("resolve runtime profile: %w", err)
		}

		webServer := profileResult.Profile.WebServer
		search := profileResult.Profile.Search
		cache := profileResult.Profile.Cache
		queue := profileResult.Profile.Queue
		dbType := profileResult.Profile.DBType
		dbVersion := profileResult.Profile.DBVersion
		phpVersion := profileResult.Profile.PHPVersion
		nodeVersion := profileResult.Profile.NodeVersion
		xdebugSession := profileResult.Profile.XdebugSession
		webRoot := profileResult.Profile.WebRoot
		enableVarnish := false

		if metadata.Framework == "custom" {
			pterm.Info.Println("Customize your stack services for the custom recipe.")
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

			phpVersion = textInput("PHP version", phpVersion)
			nodeVersion = textInput("Node.js version", nodeVersion)
			webRoot = textInput("Web root (e.g. /public, leave empty for project root)", webRoot)
			xdebugSession = textInput("Xdebug session cookie value", xdebugSession)
			enableVarnish, _ = pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show("Enable Varnish?")
		}

		config := engine.Config{
			ProjectName:      filepath.Base(cwd),
			Recipe:           metadata.Framework,
			FrameworkVersion: metadata.Version,
			Domain:           fmt.Sprintf("%s.test", filepath.Base(cwd)),
			Stack: engine.Stack{
				PHPVersion:    phpVersion,
				NodeVersion:   nodeVersion,
				DBType:        dbType,
				DBVersion:     dbVersion,
				WebRoot:       webRoot,
				XdebugSession: xdebugSession,
				Services: engine.Services{
					WebServer: webServer,
					Search:    search,
					Cache:     cache,
					Queue:     queue,
				},
				Features: engine.Features{
					Xdebug:  true,
					Varnish: enableVarnish,
				},
			},
		}

		if migrated.ProjectName != "" {
			config.ProjectName = migrated.ProjectName
			config.Domain = fmt.Sprintf("%s.test", migrated.ProjectName)
		}
		if migrated.PHPVersion != "" {
			config.Stack.PHPVersion = migrated.PHPVersion
		}
		if migrated.NodeVersion != "" {
			config.Stack.NodeVersion = migrated.NodeVersion
		}
		if migrated.DBType != "" {
			config.Stack.DBType = migrated.DBType
		}
		if migrated.DBVersion != "" {
			config.Stack.DBVersion = migrated.DBVersion
		}
		if migrated.WebRoot != "" {
			config.Stack.WebRoot = migrated.WebRoot
		}
		if migrated.SearchService != "" {
			config.Stack.Services.Search = migrated.SearchService
			config.Stack.SearchVersion = migrated.SearchVersion
		}
		if migrated.CacheService != "" {
			config.Stack.Services.Cache = migrated.CacheService
			config.Stack.CacheVersion = migrated.CacheVersion
		}
		if migrated.QueueService != "" {
			config.Stack.Services.Queue = migrated.QueueService
			config.Stack.QueueVersion = migrated.QueueVersion
		}
		if migrated.VarnishEnabled {
			config.Stack.Features.Varnish = true
			if migrated.VarnishVersion != "" {
				config.Stack.VarnishVersion = migrated.VarnishVersion
			}
		}

		if len(migrated.Remotes) > 0 {
			config.Remotes = migrated.Remotes
		}

		if config.Stack.DBType == "none" {
			config.Stack.DBVersion = ""
		}
		if hasExistingConfig {
			if existingConfig.ProjectName != "" {
				config.ProjectName = existingConfig.ProjectName
				config.Domain = existingConfig.Domain
			}
			if existingConfig.Stack.PHPVersion != "" {
				config.Stack.PHPVersion = existingConfig.Stack.PHPVersion
			}
			if existingConfig.Stack.NodeVersion != "" {
				config.Stack.NodeVersion = existingConfig.Stack.NodeVersion
			}
			if existingConfig.Stack.DBType != "" {
				config.Stack.DBType = existingConfig.Stack.DBType
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
			}
			if existingConfig.Stack.Services.Cache != "" {
				config.Stack.Services.Cache = existingConfig.Stack.Services.Cache
				config.Stack.CacheVersion = existingConfig.Stack.CacheVersion
			}
			if existingConfig.Stack.Services.Queue != "" {
				config.Stack.Services.Queue = existingConfig.Stack.Services.Queue
				config.Stack.QueueVersion = existingConfig.Stack.QueueVersion
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
		configForObservability = config
		engine.NormalizeConfig(&config)
		writableConfig := engine.PrepareConfigForWrite(config)

		data, err := yaml.Marshal(&writableConfig)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", engine.BaseConfigFile, err)
		}
		if err := os.WriteFile(engine.BaseConfigFile, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", engine.BaseConfigFile, err)
		}
		pterm.Success.Println("✅ Generated .govard.yml")

		if err := engine.RenderBlueprint(cwd, config); err != nil {
			pterm.Warning.Printf("Failed to render compose file: %v\n", err)
			pterm.Info.Println("You can retry compose rendering later via `govard up`.")
			return nil
		}
		composePath := engine.ComposeFilePath(cwd, config.ProjectName)
		pterm.Success.Printf("✅ Rendered compose file at %s\n", composePath)
		return nil
	},
}

func loadExistingInitConfig(cwd string) (engine.Config, bool) {
	configPath := filepath.Join(cwd, engine.BaseConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		return engine.Config{}, false
	}
	existing, err := engine.LoadBaseConfigFromDir(cwd, true)
	if err != nil {
		pterm.Warning.Printf("Could not load existing %s for merge: %v\n", engine.BaseConfigFile, err)
		return engine.Config{}, false
	}
	return existing, true
}

var (
	initRecipe           string
	initFrameworkVersion string
	migrateFrom          string
)

func init() {
	initCmd.Flags().StringVarP(&initRecipe, "recipe", "r", "", "Override detected framework (e.g., magento2)")
	initCmd.Flags().StringVar(&initFrameworkVersion, "framework-version", "", "Override detected framework version (e.g., 11)")
	initCmd.Flags().StringVar(&migrateFrom, "migrate-from", "", "Migrate configuration from another tool (ddev, warden)")
}

func selectOption(title string, options []string, defaultOption string) string {
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
	printer := pterm.DefaultInteractiveTextInput.WithDefaultValue(defaultValue)
	result, err := printer.Show(title)
	if err != nil {
		return defaultValue
	}
	return result
}
