package engine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"

	"github.com/pterm/pterm"
)

type magentoCommand struct {
	Desc     string
	Args     []string
	Optional bool
}

const (
	DefaultMagentoAdminUser     = conventions.DefaultAdminUser
	DefaultMagentoAdminPassword = conventions.DefaultAdminPassword
)

// MagentoConfigCommandsForTest exposes command planning for tests.
func MagentoConfigCommandsForTest(projectName string, config Config) []magentoCommand {
	return buildFrameworkAutoConfigurationCommands(projectName, config)
}

// ConfigureMagento runs post-startup Magento configuration. If shiftInfo is provided
// and indicates a shift, it will perform cleanup and reconfiguration. If shiftInfo
// is nil, it will auto-detect (legacy behavior).
func ConfigureMagento(projectName string, config Config, force bool, shiftInfo *ProfileShiftInfo) error {
	shifted := force
	reason := "manual trigger"

	if shiftInfo != nil {
		// Use pre-detected shift info from ProfileGuard stage
		shifted = shiftInfo.Shifted || force
		if shiftInfo.Reason != "" {
			reason = shiftInfo.Reason
		}
	} else if !force {
		// Legacy: auto-detect if no pre-detected info provided
		shifted, reason = checkProfileShiftCleanup(config)
	}

	if !shifted {
		return nil
	}

	pterm.Info.Printf("Configuring Magento 2 environment (%s)...\n", reason)

	if err := FixProjectPermissions(projectName, config); err != nil {
		pterm.Warning.Printf("Could not fix project permissions (continuing): %v\n", err)
	}

	if shifted {
		// Wipe Magento generated assets
		pterm.Info.Printf("Detecting runtime shift (%s). Cleaning up stale Magento assets...\n", reason)
		if wipeErr := wipeMagentoGeneratedCaches(projectName, config); wipeErr != nil {
			pterm.Warning.Printf("Could not wipe stale caches: %v\n", wipeErr)
		} else {
			pterm.Success.Println("Stale Magento assets (generated/code, var/cache) cleared.")
		}
	}

	if config.Stack.Features.Cache || config.Stack.Services.Cache != "none" {
		pterm.Info.Println("Flushing Redis cache for the new profile...")
		if redisErr := flushMagentoRedisCache(projectName, config); redisErr != nil {
			pterm.Warning.Printf("Could not flush Redis: %v\n", redisErr)
		} else {
			pterm.Success.Println("Redis cache flushed.")
		}
	}

	// Proactively unblock search index (safe via curl, not a DB query)
	if config.Stack.Features.Search || config.Stack.Services.Search != "none" {
		if err := FixElasticsearchIndexBlock(projectName, config); err != nil {
			pterm.Warning.Printf("Could not unblock search index proactively (continuing): %v", err)
		} else {
			pterm.Success.Println("Proactively unblocked search index via curl.")
		}
	}

	maybeRunMagentoComposerInstall(projectName, config)

	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	lockedKeys, _ := CheckMagentoEnvPHPLockedKeys(containerName, config)
	if len(lockedKeys) > 0 {
		pterm.Info.Printf("Detected %d locked core config keys in env.php. Govard will perform forced overrides to match local environment.\n", len(lockedKeys))
	}

	if err := ensureMagentoLocalWritableDirs(containerName, config); err != nil {
		pterm.Warning.Printf("Could not prepare Magento writable dirs (continuing): %v\n", err)
	}

	commands := buildMagento2Commands(projectName, config, lockedKeys)

	for _, cmd := range commands {
		pterm.Info.Printf("→ %s...\n", cmd.Desc)
		output, err := exec.Command("docker", cmd.Args...).CombinedOutput()
		if err != nil {
			outText := string(output)
			if IsElasticsearchIndexBlockError(outText) {
				pterm.Warning.Println("Elasticsearch/OpenSearch index is blocked (read-only); attempting to unblock...")
				if repairErr := FixElasticsearchIndexBlock(projectName, config); repairErr != nil {
					pterm.Warning.Printf("Could not unblock search index: %v\n", repairErr)
				} else {
					pterm.Success.Println("Elasticsearch/OpenSearch index unblocked.")
					// Retry once after repair.
					output, retryErr := exec.Command("docker", cmd.Args...).CombinedOutput()
					if retryErr == nil {
						continue
					}
					err = retryErr // Update err for final reporting
					outText = string(output)
				}
			}

			// After importing a database snapshot, Magento may require config import/upgrade before
			// certain CLI commands (config:set, cache, etc) are allowed.
			if needsConfigImport(outText) {
				pterm.Warning.Println("Magento requires app:config:import/setup:upgrade before continuing; attempting auto-repair...")
				_ = ensureMagentoLocalWritableDirs(containerName, config)
				if repairErr := runMagentoConfigImport(containerName, config); repairErr != nil {
					// Fallback to setup:upgrade when import isn't enough.
					pterm.Warning.Printf("app:config:import failed (%v). Trying autoloader reset and setup:upgrade...\n", repairErr)
					_ = ensureMagentoLocalWritableDirs(containerName, config)

					// Force developer mode early to allow on-the-fly generation of Proxies/Interceptors
					pterm.Info.Println("Switching to developer mode to enable on-the-fly class generation...")
					_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, conventions.BinMagento, "deploy:mode:set", "developer", "--no-interaction")...).Run()

					// Force clean generated code and caches to break the stale Interceptor/DI crash cycle
					_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", "rm -rf generated/code/* generated/metadata/* var/cache/* var/page_cache/* var/view_preprocessed/*")...).Run()

					// Reset autoloader to clear stale classmap entries that reference missing generated files
					if dumpErr := runMagentoComposerDumpAutoload(containerName, config); dumpErr != nil {
						pterm.Warning.Printf("composer dump-autoload failed (%v), continuing with setup:upgrade anyway...\n", dumpErr)
					}

					if upgradeErr := runMagentoSetupUpgrade(containerName, config); upgradeErr != nil {
						// Check if setup:upgrade failed due to search index block too
						repairOut := upgradeErr.Error()
						if IsElasticsearchIndexBlockError(repairOut) {
							pterm.Warning.Println("setup:upgrade failed due to search index block; attempting to unblock and retry...")
							if fixErr := FixElasticsearchIndexBlock(projectName, config); fixErr == nil {
								pterm.Success.Println("Elasticsearch/OpenSearch index unblocked. Retrying setup:upgrade...")
								if retryErr := runMagentoSetupUpgrade(containerName, config); retryErr == nil {
									goto retryInitialCommand
								} else {
									upgradeErr = retryErr // Update for final reporting if still fails
								}
							}
						}
						return fmt.Errorf("command failed: %s %v\nRepair attempt failed (setup:upgrade): %v\nOutput: %s\nOriginal Output: %s", cmd.Desc, err, upgradeErr, repairOut, outText)
					}
				}

			retryInitialCommand:
				// Retry once after repair.
				output, retryErr := exec.Command("docker", cmd.Args...).CombinedOutput()
				if retryErr == nil {
					continue
				}
				err = retryErr
				outText = string(output)
			}

			// Some optional settings are unavailable when the related Magento module
			// is disabled (for example TwoFactorAuth on projects that explicitly turn it off).
			// In that case skip the step without warning noise.
			if cmd.Optional && isMagentoConfigPathUnavailable(outText) {
				pterm.Info.Printf("Skipping optional Magento configure step (%s): setting path is unavailable in this project.\n", cmd.Desc)
				continue
			}

			if strings.Contains(string(output), "not found") || strings.Contains(string(output), "No such container") {
				return fmt.Errorf("container %s is not running. Run 'govard env up' first", fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix))
			}
			if cmd.Optional {
				pterm.Warning.Printf("Non-fatal Magento configure step failed (%s): %v\n", cmd.Desc, err)
				if outText != "" {
					pterm.Debug.Printf("Command output: %s\n", outText)
				}
				continue
			}
			return fmt.Errorf("command failed: %s %v\nOutput: %s", cmd.Desc, err, outText)
		}
	}

	pterm.Success.Println("Magento 2 environment configured successfully!")
	return nil
}

func needsConfigImport(output string) bool {
	output = strings.ToLower(output)

	// Traditional Magento message about config import/upgrade requirements
	if strings.Contains(output, "app:config:import") || strings.Contains(output, "setup:upgrade") {
		return true
	}

	// Class loading failures usually mean generated/code is missing or stale classmap exists.
	// This often happens after rsync sync from remote or DB import without setup:upgrade.
	if strings.Contains(output, "failed to open stream") &&
		(strings.Contains(output, "generated/code") || strings.Contains(output, "generated/metadata")) {
		return true
	}

	// Missing namespace errors (e.g. "There are no commands defined in the 'deploy:mode' namespace")
	// often happen when Magento is in a restricted command state after DB import.
	if strings.Contains(output, "no commands defined") {
		return true
	}

	return false
}

func IsElasticsearchIndexBlockError(output string) bool {
	output = strings.ToLower(output)
	return strings.Contains(output, "index_create_block_exception") ||
		strings.Contains(output, "cluster_block_exception") ||
		strings.Contains(output, "read_only_allow_delete") ||
		strings.Contains(output, "create-index blocked") ||
		(strings.Contains(output, "forbidden") && strings.Contains(output, "10")) ||
		(strings.Contains(output, "403") && (strings.Contains(output, "blocked") || strings.Contains(output, "forbidden")))
}

func FixElasticsearchIndexBlock(projectName string, config Config) error {
	containerName := fmt.Sprintf("%s-elasticsearch-1", projectName)
	// We use curl inside the elasticsearch container to reset the read-only setting.
	// This approach works for both Elasticsearch and OpenSearch.
	unblockCommand := []string{
		"exec", "-i", containerName,
		"curl", "-s", "-X", "PUT", "http://localhost:9200/_all/_settings",
		"-H", "Content-Type: application/json",
		"-d", `{"index.blocks.read_only_allow_delete": null}`,
	}

	output, err := exec.Command("docker", unblockCommand...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unblock search index: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func isMagentoConfigPathUnavailable(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	if normalized == "" {
		return false
	}

	if strings.Contains(normalized, "path") && (strings.Contains(normalized, "doesn't exist") || strings.Contains(normalized, "not found")) {
		return true
	}
	if strings.Contains(normalized, "is not defined") {
		return true
	}

	return false
}

func runMagentoConfigImport(containerName string, config Config) error {
	args := magentoDockerExecArgs(containerName, config, conventions.BinMagento, "app:config:import", "--no-interaction")
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("app:config:import failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func runMagentoSetupUpgrade(containerName string, config Config) error {
	args := magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:upgrade", "--no-interaction")
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("setup:upgrade failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func runMagentoComposerDumpAutoload(containerName string, config Config) error {
	args := magentoDockerExecArgs(containerName, config, "composer", "dump-autoload")
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("composer dump-autoload failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func ensureMagentoLocalWritableDirs(containerName string, config Config) error {
	script := strings.Join([]string{
		"set -e",
		`fix_dir() { p="$1"; if [ -L "$p" ]; then rm -f "$p"; fi; mkdir -p "$p"; }`,
		"mkdir -p generated pub/static pub/media var",
		"fix_dir var/session",
		"fix_dir var/tmp",
		"fix_dir var/report",
		"fix_dir var/import",
		"fix_dir var/export",
		"fix_dir var/import_history",
		"fix_dir var/importexport",
		"fix_dir pub/static/_cache",
		"fix_dir pub/.well-known",
	}, " && ")

	args := magentoDockerExecArgs(containerName, config, "sh", "-lc", script)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ensure writable dirs failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func buildFrameworkAutoConfigurationCommands(projectName string, config Config) []magentoCommand {
	switch strings.ToLower(config.Framework) {
	case "magento1", "openmage":
		return buildMagento1Commands(projectName, config)
	default:
		return buildMagento2Commands(projectName, config, nil)
	}
}

func buildMagento2Commands(projectName string, config Config, lockedKeys map[string]bool) []magentoCommand {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	searchEngine := ResolveMagentoSearchEngine(config)

	configSetArgs := []string{
		conventions.BinMagento,
		"setup:config:set",
		"--db-host=db",
		"--db-name=" + conventions.DefaultMagentoDBName,
		"--db-user=" + conventions.DefaultMagentoDBUser,
		"--db-password=" + conventions.DefaultMagentoDBPass,
	}
	if tablePrefix := NormalizeTablePrefix(config.TablePrefix); tablePrefix != "" {
		configSetArgs = append(configSetArgs, "--db-prefix="+tablePrefix)
	}
	configSetArgs = append(configSetArgs, "--no-interaction")

	commands := []magentoCommand{{
		Desc: "Enable Developer Mode",
		Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "deploy:mode:set", conventions.MagentoDeveloperMode, "--no-interaction"),
	}, {
		Desc: "Setting Database connection",
		Args: magentoDockerExecArgs(containerName, config, configSetArgs...),
	}}

	// Pre-fix search host in DB via CLI is handled in cmd layer
	if searchEngine != "" {
		commands = append(commands, magentoCommand{
			Desc: "Setting Search Engine",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				"catalog/search/engine", searchEngine, "--no-interaction"),
			Optional: true,
		})
		commands = append(commands, buildMagentoSearchConfigSetCommands(containerName, config, searchEngine)...)
	}

	if config.Stack.Services.Cache == conventions.ServiceRedis || config.Stack.Services.Cache == "valkey" {
		commands = append(commands, magentoCommand{
			Desc: "Configuring Redis Cache",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:config:set",
				"--cache-backend=redis",
				"--cache-backend-redis-server=redis",
				"--cache-backend-redis-db=0",
				"--page-cache=redis",
				"--page-cache-redis-server=redis",
				"--page-cache-redis-db=1",
				"--no-interaction"),
			Optional: true,
		})
		commands = append(commands, magentoCommand{
			Desc: "Configuring Redis Sessions",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:config:set",
				"--session-save=redis", "--session-save-redis-host=redis", "--session-save-redis-db=2", "--no-interaction"),
			Optional: true,
		})
	}

	if config.Stack.Services.Queue == conventions.ServiceRabbitMQ {
		commands = append(commands, magentoCommand{
			Desc: "Configuring RabbitMQ Message Queue",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:config:set",
				"--amqp-host=rabbitmq",
				"--amqp-port="+strconv.Itoa(conventions.RabbitMQPort),
				"--amqp-user=guest",
				"--amqp-password=guest",
				"--amqp-virtualhost=/",
				"--no-interaction"),
			Optional: true,
		})
	}

	if config.Stack.Features.Varnish {
		commands = append(commands, magentoCommand{
			Desc: "Configuring Varnish Page Cache",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				"system/full_page_cache/caching_application", "2", "--no-interaction"),
		})
		commands = append(commands, magentoCommand{
			Desc: "Configuring Varnish Purge Hosts",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:config:set",
				"--http-cache-hosts="+conventions.ServiceVarnish+":"+strconv.Itoa(conventions.HTTPPort), "--no-interaction"),
			Optional: true,
		})
		commands = append(commands, magentoCommand{
			Desc: "Configuring Varnish Backend Host",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				"system/full_page_cache/varnish/backend_host", "web", "--no-interaction"),
			Optional: true,
		})
		commands = append(commands, magentoCommand{
			Desc: "Configuring Varnish Backend Port",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				"system/full_page_cache/varnish/backend_port", strconv.Itoa(conventions.HTTPPort), "--no-interaction"),
			Optional: true,
		})
	}

	if config.Domain != "" {
		baseUrl := fmt.Sprintf("https://%s/", config.Domain)
		if lockedKeys["web/unsecure/base_url"] || lockedKeys["web/secure/base_url"] {
			commands = append(commands, magentoCommand{
				Desc: "Setting Base URLs (Locked in env.php)",
				Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
					"--lock-env", "web/unsecure/base_url", baseUrl, "--no-interaction"),
			}, magentoCommand{
				Desc: "Setting Secure Base URLs (Locked in env.php)",
				Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
					"--lock-env", "web/secure/base_url", baseUrl, "--no-interaction"),
			})
		} else {
			commands = append(commands, magentoCommand{
				Desc: "Setting Base URLs",
				Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "setup:store-config:set",
					"--base-url="+baseUrl, "--base-url-secure="+baseUrl, "--no-interaction"),
			})
		}

		if lockedKeys["web/cookie/cookie_domain"] {
			// Extract domain for cookie (strip subdomains if needed, or use full)
			// For local dev, using the full domain is usually safest.
			commands = append(commands, magentoCommand{
				Desc: "Setting Cookie Domain (Locked in env.php)",
				Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
					"--lock-env", "web/cookie/cookie_domain", config.Domain, "--no-interaction"),
				Optional: true,
			})
		}

		if lockedKeys["web/secure/offloader_header"] {
			commands = append(commands, magentoCommand{
				Desc: "Setting Offloader Header (Locked in env.php)",
				Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
					"--lock-env", "web/secure/offloader_header", "X-Forwarded-Proto", "--no-interaction"),
				Optional: true,
			})
		}
	}

	// Per-store base URLs
	for domain, mapping := range config.StoreDomains {
		scopeCode := mapping.ScopeCode()
		if scopeCode == "" {
			continue
		}
		baseURL := fmt.Sprintf("https://%s/", domain)
		scopeFlag := "--scope=stores"
		scopeDesc := "store"
		if mapping.ScopeType() == "website" {
			scopeFlag = "--scope=websites"
			scopeDesc = "website"
		}
		commands = append(commands, magentoCommand{
			Desc: fmt.Sprintf("Setting Base URL for %s %s", scopeDesc, scopeCode),
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				scopeFlag, "--scope-code="+scopeCode, "web/unsecure/base_url", baseURL, "--no-interaction"),
			Optional: true,
		}, magentoCommand{
			Desc: fmt.Sprintf("Setting Secure Base URL for %s %s", scopeDesc, scopeCode),
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				scopeFlag, "--scope-code="+scopeCode, "web/secure/base_url", baseURL, "--no-interaction"),
			Optional: true,
		})
	}

	commands = append(commands, magentoCommand{
		Desc: "Enable Web Server Rewrites",
		Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
			"web/seo/use_rewrites", "1", "--no-interaction"),
		Optional: true,
	})

	commands = append(commands, magentoCommand{
		Desc: "Disable reCAPTCHA",
		Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
			"recaptcha_frontend/type_for/customer_login", "invisible", "--no-interaction"),
		Optional: true,
	})
	commands = append(commands, magentoCommand{
		Desc: "Disable 2FA",
		Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
			"twofactorauth/general/enable", "0", "--no-interaction"),
		Optional: true,
	})

	if config.Stack.Features.LiveReload {
		lrScript := `<script src="http://localhost:35729/livereload.js?snipver=1"></script>`
		// Using --lock-env to write to app/etc/env.php as requested (prevents DB pollution).
		commands = append(commands, magentoCommand{
			Desc: "Injecting LiveReload script into env.php footer",
			Args: magentoDockerExecArgs(containerName, config, conventions.BinMagento, "config:set",
				"--lock-env", "design/footer/absolute_footer", lrScript, "--no-interaction"),
			Optional: true,
		})
	}

	return commands
}

func ConfigureMagento1(projectName string, config Config) error {
	pterm.Info.Println("Configuring Magento 1 environment...")

	commands := buildMagento1Commands(projectName, config)
	for _, cmd := range commands {
		pterm.Info.Printf("→ %s...\n", cmd.Desc)
		output, err := exec.Command("docker", cmd.Args...).CombinedOutput()
		if err != nil {
			if cmd.Optional {
				pterm.Warning.Printf("Non-fatal Magento 1 configure step failed (%s): %v\n", cmd.Desc, err)
				continue
			}
			if strings.Contains(string(output), "No such container") {
				return fmt.Errorf("container %s is not running. Run 'govard env up' first", fmt.Sprintf("%s%s", projectName, conventions.DBSuffix))
			}
			return fmt.Errorf("command failed: %s %v\nOutput: %s", cmd.Desc, err, string(output))
		}
	}

	pterm.Success.Println("Magento 1 environment configured successfully!")
	return nil
}

func buildMagento1Commands(projectName string, config Config) []magentoCommand {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.DBSuffix)
	commands := make([]magentoCommand, 0)
	tablePrefix := NormalizeTablePrefix(config.TablePrefix)

	if config.Domain != "" {
		baseURL := fmt.Sprintf("https://%s/", config.Domain)
		sqlStatements := BuildMagento1SetConfigSQLStatements(baseURL, tablePrefix)
		for idx, sql := range sqlStatements {
			commands = append(commands, magentoCommand{
				Desc: fmt.Sprintf("Setting Magento 1 base configuration (%d/%d)", idx+1, len(sqlStatements)),
				Args: magento1DockerSQLExecArgs(containerName, sql),
			})
		}
	}

	for domain, mapping := range config.StoreDomains {
		scopeCode := mapping.ScopeCode()
		if scopeCode == "" {
			continue
		}
		baseURL := fmt.Sprintf("https://%s/", domain)
		var sqlStatements []string
		switch mapping.ScopeType() {
		case "website":
			sqlStatements = BuildMagento1WebsiteBaseURLSQLStatements(scopeCode, baseURL, tablePrefix)
		case "store":
			sqlStatements = BuildMagento1StoreBaseURLSQLStatements(scopeCode, baseURL, tablePrefix)
		default:
			sqlStatements = BuildMagento1ScopedBaseURLSQLStatements(scopeCode, baseURL, tablePrefix)
		}
		for idx, sql := range sqlStatements {
			commands = append(commands, magentoCommand{
				Desc:     fmt.Sprintf("Setting Magento 1 scoped base URL for %s (%d/%d)", scopeCode, idx+1, len(sqlStatements)),
				Args:     magento1DockerSQLExecArgs(containerName, sql),
				Optional: true,
			})
		}
	}

	return commands
}

func buildMagentoSearchConfigSetCommands(containerName string, config Config, engineName string) []magentoCommand {
	prefix := resolveMagentoSearchConfigPrefix(engineName)
	if prefix == "" {
		return nil
	}

	settings := []struct {
		desc  string
		path  string
		value string
	}{
		{desc: "Setting Search Host", path: "catalog/search/" + prefix + "_server_hostname", value: conventions.ServiceElasticsearch},
		{desc: "Setting Search Port", path: "catalog/search/" + prefix + "_server_port", value: "9200"},
		{desc: "Setting Search Index Prefix", path: "catalog/search/" + prefix + "_index_prefix", value: conventions.DefaultMagentoDBName},
		{desc: "Setting Search Auth", path: "catalog/search/" + prefix + "_enable_auth", value: "0"},
		{desc: "Setting Search Timeout", path: "catalog/search/" + prefix + "_server_timeout", value: "15"},
	}

	commands := make([]magentoCommand, 0, len(settings))
	for _, setting := range settings {
		commands = append(commands, magentoCommand{
			Desc: setting.desc,
			Args: magentoDockerExecArgs(
				containerName,
				config,
				conventions.BinMagento,
				"config:set",
				setting.path,
				setting.value,
				"--no-interaction",
			),
			Optional: true,
		})
	}
	return commands
}

func resolveMagentoSearchConfigPrefix(engineName string) string {
	switch engineName {
	case conventions.ServiceOpenSearch:
		return conventions.ServiceOpenSearch
	case "elasticsearch7":
		return "elasticsearch7"
	default:
		return ""
	}
}

func ResolveMagentoSearchEngine(config Config) string {
	// ElasticSuite must remain the selected engine when the module is present.
	// Forcing elasticsearch7/opensearch breaks Smile query objects on Magento 2.4.7 stacks.
	if isMagentoElasticsuiteProject() {
		return "elasticsuite"
	}

	search := strings.ToLower(strings.TrimSpace(config.Stack.Services.Search))
	if search == "" || search == "none" {
		return ""
	}

	// Magento < 2.4.8 uses the elasticsearch7 engine name/flags even when running OpenSearch.
	if isMagentoVersionAtLeast(config.FrameworkVersion, "2.4.8") && search == conventions.ServiceOpenSearch {
		return conventions.ServiceOpenSearch
	}
	return "elasticsearch7"
}

func isMagentoElasticsuiteProject() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	elasticSuiteCore := filepath.Join(cwd, "vendor", "smile", "elasticsuite", "src", "module-elasticsuite-core")
	if info, statErr := os.Stat(elasticSuiteCore); statErr == nil && info.IsDir() {
		return true
	}

	configPHP := filepath.Join(cwd, "app", "etc", "config.php")
	if data, readErr := os.ReadFile(configPHP); readErr == nil {
		if strings.Contains(string(data), "'Smile_ElasticsuiteCore' => 1") {
			return true
		}
	}

	composerJSON := filepath.Join(cwd, "composer.json")
	if data, readErr := os.ReadFile(composerJSON); readErr == nil {
		if strings.Contains(string(data), "smile/elasticsuite") {
			return true
		}
	}

	return false
}

func isMagentoVersionAtLeast(raw string, minimum string) bool {
	return IsNumericDotVersionAtLeast(raw, minimum)
}

// BuildMagentoSearchHostFixSQL returns the SQL query needed to fix the search host in the database.
// It is used by the CLI (govard db query) during bootstrap or auto-configuration.
func BuildMagentoSearchHostFixSQL(host string, searchEngine string) string {
	if host == "" {
		host = conventions.ServiceElasticsearch
	}
	// Query information_schema to handle potential table prefixes
	sql := "SET @table_name = (SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE '%core_config_data' LIMIT 1); "

	// 1. Fix Hostname
	sql += fmt.Sprintf("SET @sql = IF(@table_name IS NOT NULL, CONCAT('UPDATE ', @table_name, ' SET value = \"%s\" WHERE path IN (\"catalog/search/elasticsearch_server_hostname\", \"catalog/search/elasticsearch5_server_hostname\", \"catalog/search/elasticsearch6_server_hostname\", \"catalog/search/elasticsearch7_server_hostname\", \"catalog/search/opensearch_server_hostname\")'), 'SELECT 1'); ", host)
	sql += "PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt; "

	// 2. Fix Port (Default to 9200 for local containers)
	sql += "SET @sql_port = IF(@table_name IS NOT NULL, CONCAT('UPDATE ', @table_name, ' SET value = \"9200\" WHERE path IN (\"catalog/search/elasticsearch5_server_port\", \"catalog/search/elasticsearch6_server_port\", \"catalog/search/elasticsearch7_server_port\", \"catalog/search/opensearch_server_port\")'), 'SELECT 1'); "
	sql += "PREPARE stmt_port FROM @sql_port; EXECUTE stmt_port; DEALLOCATE PREPARE stmt_port; "

	// 3. Disable Authentication (Prevent issues if remote uses auth)
	sql += "SET @sql_auth = IF(@table_name IS NOT NULL, CONCAT('UPDATE ', @table_name, ' SET value = \"0\" WHERE path IN (\"catalog/search/elasticsearch5_enable_auth\", \"catalog/search/elasticsearch6_enable_auth\", \"catalog/search/elasticsearch7_enable_auth\", \"catalog/search/opensearch_enable_auth\", \"smile_elasticsuite_core_base_settings/es_client/enable_auth\")'), 'SELECT 1'); "
	sql += "PREPARE stmt_auth FROM @sql_auth; EXECUTE stmt_auth; DEALLOCATE PREPARE stmt_auth; "

	// 4. Smile_Elasticsuite specific fix (uses host:port format)
	sql += fmt.Sprintf("SET @sql2 = IF(@table_name IS NOT NULL, CONCAT('UPDATE ', @table_name, ' SET value = \"%s:9200\" WHERE path = \"smile_elasticsuite_core_base_settings/es_client/servers\"'), 'SELECT 1'); ", host)
	sql += "PREPARE stmt2 FROM @sql2; EXECUTE stmt2; DEALLOCATE PREPARE stmt2;"

	// 5. Fix Engine Type
	if searchEngine != "" {
		sql += fmt.Sprintf("SET @sql_engine = IF(@table_name IS NOT NULL, CONCAT('INSERT INTO ', @table_name, ' (scope, scope_id, path, value) VALUES (''default'', 0, ''catalog/search/engine'', ''%s'') ON DUPLICATE KEY UPDATE value = ''%s'''), 'SELECT 1'); ", searchEngine, searchEngine)
		sql += "PREPARE stmt_engine FROM @sql_engine; EXECUTE stmt_engine; DEALLOCATE PREPARE stmt_engine;"
	}

	return sql
}

func magentoDockerExecArgs(containerName string, config Config, args ...string) []string {
	result := []string{"exec"}
	if user := resolveMagentoExecUser(config); strings.TrimSpace(user) != "" {
		result = append(result, "-u", user)
	}
	result = append(result, "-w", conventions.DefaultWorkDir, containerName)
	result = append(result, args...)
	return result
}

func magento1DockerSQLExecArgs(containerName string, sql string) []string {
	script := fmt.Sprintf(
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else exit 1; fi && echo %s | "$DB_CLI" -u %s %s -f`,
		ShellQuote(sql), ShellQuote(conventions.DefaultMagentoDBUser), ShellQuote(conventions.DefaultMagentoDBName),
	)

	return []string{
		"exec", "-i",
		"-e", "MYSQL_PWD=" + conventions.DefaultMagentoDBPass,
		containerName,
		"sh", "-lc", script,
	}
}

func resolveMagentoExecUser(config Config) string {
	if config.Stack.UserID > 0 && config.Stack.GroupID > 0 {
		return fmt.Sprintf("%d:%d", config.Stack.UserID, config.Stack.GroupID)
	}
	return conventions.UserWWWData
}

func FixProjectPermissions(projectName string, config Config) error {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	if len(config.Stack.ChownDirList) == 0 {
		return nil
	}

	pterm.Info.Printf("Ensuring correct permissions for project directories...\n")

	dirs := ""
	for _, d := range config.Stack.ChownDirList {
		dirs += fmt.Sprintf("%s ", ShellQuote(d))
	}

	// We build a robust shell loop that forces root privileges and complies with Adobe best practices (including bin/magento executable).
	user := resolveMagentoExecUser(config)
	script := fmt.Sprintf("for d in %s; do if [ -e \"$d\" ]; then chown -R %s \"$d\" && chmod -R u+rwX \"$d\"; fi; done && if [ -f bin/magento ]; then chmod +x bin/magento; fi", dirs, user)

	// We MUST run as root to have permission to change file ownership.
	args := []string{"exec", "-u", "root", "-w", conventions.DefaultWorkDir, containerName, "sh", "-lc", script}
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("fix permissions failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// IsMagentoElasticsuiteProjectForTest exposes isMagentoElasticsuiteProject for testing in /tests.
func IsMagentoElasticsuiteProjectForTest() bool {
	return isMagentoElasticsuiteProject()
}

// IsMagentoConfigPathUnavailableForTest exposes isMagentoConfigPathUnavailable for testing in /tests.
func IsMagentoConfigPathUnavailableForTest(output string) bool {
	return isMagentoConfigPathUnavailable(output)
}

// CheckMagentoEnvPHPLockedKeys inspects app/etc/env.php for keys that are hardcoded (locked).
func CheckMagentoEnvPHPLockedKeys(containerName string, config Config) (map[string]bool, error) {
	// PHP snippet to check for locked keys in env.php
	script := `
$env = @include 'app/etc/env.php';
$keys = [
    'web/unsecure/base_url' => ['system', 'default', 'web', 'unsecure', 'base_url'],
    'web/secure/base_url' => ['system', 'default', 'web', 'secure', 'base_url'],
    'web/cookie/cookie_domain' => ['system', 'default', 'web', 'cookie', 'cookie_domain'],
    'web/secure/offloader_header' => ['system', 'default', 'web', 'secure', 'offloader_header'],
];
$found = [];
if (is_array($env)) {
    foreach ($keys as $name => $path) {
        $val = $env;
        foreach ($path as $p) {
            $val = $val[$p] ?? null;
        }
        if ($val !== null) {
            $found[] = $name;
        }
    }
}
echo implode(',', $found);
`
	args := magentoDockerExecArgs(containerName, config, "php", "-r", script)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return nil, nil // Silently fail if php/env.php unavailable
	}

	results := make(map[string]bool)
	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	for _, p := range parts {
		if p != "" {
			results[p] = true
		}
	}
	return results, nil
}

func wipeMagentoGeneratedCaches(projectName string, config Config) error {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)
	// We wipe generated and cache dirs.
	// rm -rf is safe because ensureMagentoLocalWritableDirs will recreate them if needed.
	script := "rm -rf generated/code/* generated/metadata/* var/cache/* var/page_cache/* var/view_preprocessed/*"
	args := magentoDockerExecArgs(containerName, config, "sh", "-c", script)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("wipe failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// runMagentoComposerInstallFn is a package-level indirection so tests can observe/stub
// composer-install invocations without shelling out to docker.
var runMagentoComposerInstallFn = runMagentoComposerInstall

// SetMagentoComposerInstallRunnerForTest overrides the composer-install runner used by
// maybeRunMagentoComposerInstall for tests.
func SetMagentoComposerInstallRunnerForTest(fn func(projectName string, config Config, stdout, stderr io.Writer) error) func() {
	prev := runMagentoComposerInstallFn
	runMagentoComposerInstallFn = fn
	return func() {
		runMagentoComposerInstallFn = prev
	}
}

// MaybeRunMagentoComposerInstallForTest exposes maybeRunMagentoComposerInstall for tests in /tests.
func MaybeRunMagentoComposerInstallForTest(projectName string, config Config) {
	maybeRunMagentoComposerInstall(projectName, config)
}

// maybeRunMagentoComposerInstall runs composer install for the given project unless the
// current working directory's vendor/ already satisfies composer.lock, in which case it is
// skipped (this is what previously made `govard config auto` retry a private-repo
// authentication failure that a remote vendor-sync fallback had already recovered from).
func maybeRunMagentoComposerInstall(projectName string, config Config) {
	skipComposerInstall := false
	if projectRoot, rootErr := os.Getwd(); rootErr == nil {
		skipComposerInstall, _ = VendorSatisfiesComposerLock(projectRoot)
	}

	if skipComposerInstall {
		pterm.Info.Println("vendor/ already satisfies composer.lock. Skipping composer install.")
		return
	}

	pterm.Info.Println("Running composer install for the new profile environment...")
	if compErr := runMagentoComposerInstallFn(projectName, config, nil, nil); compErr != nil {
		pterm.Warning.Printf("Composer install failed (continuing): %v\n", compErr)
	} else {
		pterm.Success.Println("Composer dependencies synchronized.")
	}
}

func runMagentoComposerInstall(projectName string, config Config, stdout, stderr io.Writer) error {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.PHPSuffix)

	// Protect the current (presumably working) vendor/composer + vendor/autoload.php
	// before anything below has a chance to wipe them, so a failed install can be
	// rolled back instead of leaving the environment without an autoloader.
	backupMagentoComposerRuntime(containerName, config)

	// Pre-cleanup: Wipe generated code to ensure a clean state for the autoloader.
	// This prevents stale class map entries if we run composer dump-autoload --optimize later.
	pterm.Debug.Println("Cleaning generated code before installation...")
	_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", "rm -rf generated/code/* generated/metadata/*")...).Run()

	// Two-phase composer install to avoid "Cannot redeclare class" fatal errors.
	//
	// Root cause: When switching between Magento profiles (e.g. 2.4.6-p3 → 2.4.8-p4),
	// the vendor/ directory contains stale Composer plugin files (dealerdirect/phpcodesniffer-composer-installer,
	// phpcsstandards/composer-installer, magento/composer-dependency-version-audit-plugin, etc.).
	// When Composer starts, it activates these plugins by loading their classes via the old autoloader.
	// If the new composer.lock requires different versions of the same plugins, Composer downloads
	// the new files into vendor/ mid-operation, but the old classes are already in memory.
	// When the new autoloader tries to load the same class from the updated path → Fatal Error.
	//
	// Fix: Phase 1 runs `composer install --no-plugins --no-scripts` which completely bypasses
	// plugin class loading. This safely updates all packages in vendor/ including the plugin
	// packages themselves. Phase 2 runs `composer dump-autoload` which regenerates the autoloader
	// with the now-correct plugin files, and activates plugins against a clean state.

	// Phase 1: Install packages without loading plugins or running scripts.
	// This avoids loading stale plugin classes that would conflict with newly downloaded versions.
	phase1Script := strings.Join([]string{
		"rm -rf vendor/composer",
		"composer install --no-interaction --no-progress --no-plugins --no-scripts --ignore-platform-reqs",
	}, " && ")
	phase1Args := magentoDockerExecArgs(containerName, config, "sh", "-c", phase1Script)

	pterm.Debug.Println("Phase 1: Installing packages (no-plugins, no-scripts)...")
	var phase1Buf bytes.Buffer
	phase1Cmd := exec.Command("docker", phase1Args...)

	if stdout != nil {
		phase1Cmd.Stdout = io.MultiWriter(stdout, &phase1Buf)
	} else {
		phase1Cmd.Stdout = &phase1Buf
	}
	if stderr != nil {
		phase1Cmd.Stderr = io.MultiWriter(stderr, &phase1Buf)
	} else {
		phase1Cmd.Stderr = &phase1Buf
	}

	err := phase1Cmd.Run()
	if err != nil {
		outText := phase1Buf.String()

		// If phase 1 still fails (e.g. corrupted vendor/autoload.php from an old crash),
		// try harder: wipe the entire vendor/composer + vendor/autoload.php
		if strings.Contains(outText, "Cannot redeclare") || strings.Contains(outText, "Fatal error") {
			pterm.Warning.Println("Phase 1 hit autoloader corruption. Cleaning vendor/autoload.php and retrying...")
			cleanScript := "rm -rf vendor/composer vendor/autoload.php"
			_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", cleanScript)...).Run()

			var retryBuf bytes.Buffer
			phase1RetryCmd := exec.Command("docker", phase1Args...)
			if stdout != nil {
				phase1RetryCmd.Stdout = io.MultiWriter(stdout, &retryBuf)
			} else {
				phase1RetryCmd.Stdout = &retryBuf
			}
			if stderr != nil {
				phase1RetryCmd.Stderr = io.MultiWriter(stderr, &retryBuf)
			} else {
				phase1RetryCmd.Stderr = &retryBuf
			}

			if retryErr := phase1RetryCmd.Run(); retryErr != nil {
				restoreMagentoComposerRuntimeBackup(containerName, config)
				return fmt.Errorf("composer install (phase 1 retry) failed: %w\nOutput: %s", retryErr, retryBuf.String())
			}
		} else {
			restoreMagentoComposerRuntimeBackup(containerName, config)
			return fmt.Errorf("composer install (phase 1) failed: %w\nOutput: %s", err, outText)
		}
	}

	// Phase 1 succeeded (either on the first attempt or after the corruption retry):
	// the backup taken above is no longer needed.
	discardMagentoComposerRuntimeBackup(containerName, config)

	// Phase 2: Regenerate the autoloader with the updated plugin files.
	// This runs scripts and activates plugins against a now-clean vendor/ state.
	phase2Script := "composer dump-autoload --no-interaction"
	phase2Args := magentoDockerExecArgs(containerName, config, "sh", "-c", phase2Script)

	pterm.Debug.Println("Phase 2: Regenerating autoloader with plugins...")
	var phase2Buf bytes.Buffer
	phase2Cmd := exec.Command("docker", phase2Args...)
	if stdout != nil {
		phase2Cmd.Stdout = io.MultiWriter(stdout, &phase2Buf)
	} else {
		phase2Cmd.Stdout = &phase2Buf
	}
	if stderr != nil {
		phase2Cmd.Stderr = io.MultiWriter(stderr, &phase2Buf)
	} else {
		phase2Cmd.Stderr = &phase2Buf
	}

	if err := phase2Cmd.Run(); err != nil {
		// Phase 2 failure is non-fatal: the packages are installed, just the autoloader
		// might not be fully optimized. Magento will still work.
		pterm.Warning.Printf("composer dump-autoload failed (non-fatal): %v\n", err)
	}

	return nil
}

// backupMagentoComposerRuntime moves the current vendor/composer directory and
// vendor/autoload.php aside before composer install runs, so a failed install can be
// rolled back instead of leaving the environment without an autoloader. Best-effort:
// if this fails, runMagentoComposerInstall proceeds without a safety net, exactly as
// it did before this existed.
func backupMagentoComposerRuntime(containerName string, config Config) {
	script := `rm -rf vendor/.composer-backup
mkdir -p vendor/.composer-backup
if [ -d vendor/composer ]; then mv vendor/composer vendor/.composer-backup/composer; fi
if [ -f vendor/autoload.php ]; then mv vendor/autoload.php vendor/.composer-backup/autoload.php; fi`
	_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", script)...).Run()
}

// restoreMagentoComposerRuntimeBackup undoes backupMagentoComposerRuntime: it discards
// whatever partial vendor/composer state a failed composer install left behind and
// restores the pre-attempt vendor/composer and vendor/autoload.php. Best-effort.
func restoreMagentoComposerRuntimeBackup(containerName string, config Config) {
	script := `if [ -d vendor/.composer-backup ]; then
  rm -rf vendor/composer vendor/autoload.php
  if [ -d vendor/.composer-backup/composer ]; then mv vendor/.composer-backup/composer vendor/composer; fi
  if [ -f vendor/.composer-backup/autoload.php ]; then mv vendor/.composer-backup/autoload.php vendor/autoload.php; fi
fi`
	_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", script)...).Run()
}

// discardMagentoComposerRuntimeBackup removes the backup taken by
// backupMagentoComposerRuntime once composer install has succeeded and the backup is
// no longer needed. Best-effort.
func discardMagentoComposerRuntimeBackup(containerName string, config Config) {
	_ = exec.Command("docker", magentoDockerExecArgs(containerName, config, "sh", "-c", "rm -rf vendor/.composer-backup")...).Run()
}

func flushMagentoRedisCache(projectName string, config Config) error {
	containerName := fmt.Sprintf("%s%s", projectName, conventions.RedisSuffix)

	// 1. Wait for Redis/Valkey to be ready (up to 10 seconds)
	// This prevents race conditions where the container is started but the server is not yet accepting connections.
	maxRetries := 20
	ready := false
	var lastErr error

	pterm.Debug.Printf("Waiting for cache container %s to be ready...\n", containerName)

	for i := 0; i < maxRetries; i++ {
		// 1. Check if container is running
		inspectArgs := []string{"inspect", "-f", "{{.State.Running}}", containerName}
		runningOutput, inspectErr := exec.Command("docker", inspectArgs...).CombinedOutput()
		if inspectErr != nil || strings.TrimSpace(string(runningOutput)) != "true" {
			pterm.Debug.Printf("Container %s is not running (attempt %d/%d)\n", containerName, i+1, maxRetries)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 2. Try redis-cli ping
		args := []string{"exec", containerName, "redis-cli", "ping"}
		output, err := exec.Command("docker", args...).CombinedOutput()
		if err == nil && strings.Contains(strings.ToLower(string(output)), "pong") {
			ready = true
			break
		}

		// 3. Fallback for Valkey-only images if redis-cli symlink is missing
		args = []string{"exec", containerName, "valkey-cli", "ping"}
		output, err = exec.Command("docker", args...).CombinedOutput()
		if err == nil && strings.Contains(strings.ToLower(string(output)), "pong") {
			ready = true
			break
		}

		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	if !ready {
		// If not ready, check if it's exited and why
		inspectArgs := []string{"inspect", "-f", "{{.State.Status}} (ExitCode: {{.State.ExitCode}})", containerName}
		statusOutput, _ := exec.Command("docker", inspectArgs...).CombinedOutput()
		return fmt.Errorf("cache container %s not ready: %v (Status: %s)", containerName, lastErr, strings.TrimSpace(string(statusOutput)))
	}

	// 2. Perform flushall
	// Try redis-cli first
	args := []string{"exec", containerName, "redis-cli", "flushall"}
	_, err := exec.Command("docker", args...).CombinedOutput()
	if err == nil {
		return nil
	}

	// Fallback to valkey-cli
	args = []string{"exec", containerName, "valkey-cli", "flushall"}
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis/valkey flush failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func prepareMagentoRunMappingAssets(config Config) (string, string, error) {
	if !isMagentoFramework(config.Framework) || strings.TrimSpace(config.ProjectName) == "" {
		return "", "", nil
	}

	nginxPath := filepath.Join(GovardHomeDir(), "nginx", config.ProjectName, "mage-run-map.conf")
	apachePath := filepath.Join(GovardHomeDir(), "apache", config.ProjectName, "mage-run-map.conf")

	if err := os.MkdirAll(filepath.Dir(nginxPath), conventions.DefaultDirPerm); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(apachePath), conventions.DefaultDirPerm); err != nil {
		return "", "", err
	}

	if err := os.WriteFile(nginxPath, []byte(buildMagentoNginxRunMap(config.StoreDomains)), conventions.DefaultFilePerm); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(apachePath, []byte(buildMagentoApacheRunMap(config.StoreDomains)), conventions.DefaultFilePerm); err != nil {
		return "", "", err
	}

	return nginxPath, apachePath, nil
}

func isMagentoFramework(framework string) bool {
	switch strings.ToLower(strings.TrimSpace(framework)) {
	case "magento1", "magento2", "openmage":
		return true
	default:
		return false
	}
}

func buildMagentoNginxRunMap(mappings StoreDomainMappings) string {
	lines := []string{
		"map $host $mage_run_code {",
		`    default "";`,
	}

	typedHosts := sortedStoreDomainHosts(mappings)
	for _, host := range typedHosts {
		mapping := mappings[host]
		if mapping.ScopeType() == "" || mapping.ScopeCode() == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %s %s;", host, mapping.ScopeCode()))
	}
	lines = append(lines, "}", "", "map $host $mage_run_type {", `    default "";`)
	for _, host := range typedHosts {
		mapping := mappings[host]
		if mapping.ScopeType() == "" || mapping.ScopeCode() == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %s %s;", host, mapping.ScopeType()))
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
}

func buildMagentoApacheRunMap(mappings StoreDomainMappings) string {
	lines := []string{"# Generated by Govard"}
	for _, host := range sortedStoreDomainHosts(mappings) {
		mapping := mappings[host]
		if mapping.ScopeType() == "" || mapping.ScopeCode() == "" {
			continue
		}
		hostPattern := "^" + regexp.QuoteMeta(host) + "(?::\\d+)?$"
		lines = append(lines,
			fmt.Sprintf(`SetEnvIfNoCase Host "%s" MAGE_RUN_CODE=%s`, hostPattern, mapping.ScopeCode()),
			fmt.Sprintf(`SetEnvIfNoCase Host "%s" MAGE_RUN_TYPE=%s`, hostPattern, mapping.ScopeType()),
		)
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func BuildMagento1SetConfigSQLStatements(baseURL string, dbPrefix string) []string {
	return []string{
		fmt.Sprintf("UPDATE %score_config_data SET value = '%s' WHERE path IN ('web/secure/base_url', 'web/unsecure/base_url')", dbPrefix, baseURL),
		"UPDATE " + dbPrefix + "core_config_data SET value = '{{secure_base_url}}' WHERE path IN ('web/unsecure/base_link_url', 'web/secure/base_link_url')",
		"UPDATE " + dbPrefix + "core_config_data SET value = '{{secure_base_url}}skin/' WHERE path IN ('web/unsecure/base_skin_url', 'web/secure/base_skin_url')",
		"UPDATE " + dbPrefix + "core_config_data SET value = '{{secure_base_url}}media/' WHERE path IN ('web/unsecure/base_media_url', 'web/secure/base_media_url')",
		"UPDATE " + dbPrefix + "core_config_data SET value = '{{secure_base_url}}js/' WHERE path IN ('web/unsecure/base_js_url', 'web/secure/base_js_url')",
		"UPDATE " + dbPrefix + "core_config_data SET value = 'HTTP_X_FORWARDED_PROTO' WHERE path = 'web/secure/offloader_header'",
		"UPDATE " + dbPrefix + "core_config_data SET value = '1' WHERE path = 'web/secure/use_in_frontend'",
		"UPDATE " + dbPrefix + "core_config_data SET value = '1' WHERE path = 'web/secure/use_in_adminhtml'",
		"UPDATE " + dbPrefix + "core_config_data SET value = '0' WHERE path = 'web/url/redirect_to_base'",
		"UPDATE " + dbPrefix + "core_config_data SET value = NULL WHERE path = 'web/cookie/cookie_domain'",
		"UPDATE " + dbPrefix + "core_config_data SET value = '/' WHERE path = 'web/cookie/cookie_path'",
	}
}

func BuildMagento1ScopedBaseURLSQLStatements(scopeCode string, baseURL string, dbPrefix string) []string {
	statements := BuildMagento1WebsiteBaseURLSQLStatements(scopeCode, baseURL, dbPrefix)
	statements = append(statements, BuildMagento1StoreBaseURLSQLStatements(scopeCode, baseURL, dbPrefix)...)
	return statements
}

func BuildMagento1WebsiteBaseURLSQLStatements(scopeCode string, baseURL string, dbPrefix string) []string {
	scopeCodeSQL := conventions.ShellQuote(scopeCode)
	baseURLSQL := conventions.ShellQuote(baseURL)
	configTable := dbPrefix + "core_config_data"
	scopeTable := dbPrefix + "core_website"
	return []string{
		fmt.Sprintf(
			"UPDATE %s cfg JOIN %s scope_entity ON scope_entity.website_id = cfg.scope_id SET cfg.value = %s WHERE cfg.scope = 'websites' AND scope_entity.code = %s AND cfg.path = 'web/unsecure/base_url'",
			configTable, scopeTable, baseURLSQL, scopeCodeSQL,
		),
		fmt.Sprintf(
			"INSERT INTO %s (scope, scope_id, path, value) SELECT 'websites', scope_entity.website_id, 'web/unsecure/base_url', %s FROM %s scope_entity WHERE scope_entity.code = %s AND NOT EXISTS (SELECT 1 FROM %s cfg WHERE cfg.scope = 'websites' AND cfg.scope_id = scope_entity.website_id AND cfg.path = 'web/unsecure/base_url')",
			configTable, baseURLSQL, scopeTable, scopeCodeSQL, configTable,
		),
		fmt.Sprintf(
			"UPDATE %s cfg JOIN %s scope_entity ON scope_entity.website_id = cfg.scope_id SET cfg.value = %s WHERE cfg.scope = 'websites' AND scope_entity.code = %s AND cfg.path = 'web/secure/base_url'",
			configTable, scopeTable, baseURLSQL, scopeCodeSQL,
		),
		fmt.Sprintf(
			"INSERT INTO %s (scope, scope_id, path, value) SELECT 'websites', scope_entity.website_id, 'web/secure/base_url', %s FROM %s scope_entity WHERE scope_entity.code = %s AND NOT EXISTS (SELECT 1 FROM %s cfg WHERE cfg.scope = 'websites' AND cfg.scope_id = scope_entity.website_id AND cfg.path = 'web/secure/base_url')",
			configTable, baseURLSQL, scopeTable, scopeCodeSQL, configTable,
		),
	}
}

func BuildMagento1StoreBaseURLSQLStatements(scopeCode string, baseURL string, dbPrefix string) []string {
	scopeCodeSQL := conventions.ShellQuote(scopeCode)
	baseURLSQL := conventions.ShellQuote(baseURL)
	configTable := dbPrefix + "core_config_data"
	scopeTable := dbPrefix + "core_store"
	return []string{
		fmt.Sprintf(
			"UPDATE %s cfg JOIN %s scope_entity ON scope_entity.store_id = cfg.scope_id SET cfg.value = %s WHERE cfg.scope = 'stores' AND scope_entity.code = %s AND cfg.path = 'web/unsecure/base_url'",
			configTable, scopeTable, baseURLSQL, scopeCodeSQL,
		),
		fmt.Sprintf(
			"INSERT INTO %s (scope, scope_id, path, value) SELECT 'stores', scope_entity.store_id, 'web/unsecure/base_url', %s FROM %s scope_entity WHERE scope_entity.code = %s AND NOT EXISTS (SELECT 1 FROM %s cfg WHERE cfg.scope = 'stores' AND cfg.scope_id = scope_entity.store_id AND cfg.path = 'web/unsecure/base_url')",
			configTable, baseURLSQL, scopeTable, scopeCodeSQL, configTable,
		),
		fmt.Sprintf(
			"UPDATE %s cfg JOIN %s scope_entity ON scope_entity.store_id = cfg.scope_id SET cfg.value = %s WHERE cfg.scope = 'stores' AND scope_entity.code = %s AND cfg.path = 'web/secure/base_url'",
			configTable, scopeTable, baseURLSQL, scopeCodeSQL,
		),
		fmt.Sprintf(
			"INSERT INTO %s (scope, scope_id, path, value) SELECT 'stores', scope_entity.store_id, 'web/secure/base_url', %s FROM %s scope_entity WHERE scope_entity.code = %s AND NOT EXISTS (SELECT 1 FROM %s cfg WHERE cfg.scope = 'stores' AND cfg.scope_id = scope_entity.store_id AND cfg.path = 'web/secure/base_url')",
			configTable, conventions.ShellQuote(baseURL), scopeTable, conventions.ShellQuote(scopeCode), configTable,
		),
	}
}

// CheckProfileShiftCleanupForTest exposes checkProfileShiftCleanup for testing.
func CheckProfileShiftCleanupForTest(config Config) (bool, string) {
	return checkProfileShiftCleanup(config)
}

// DetectProfileShiftForTest exposes DetectProfileShift for testing.
func DetectProfileShiftForTest(config Config) ProfileShiftInfo {
	return DetectProfileShift(config)
}
