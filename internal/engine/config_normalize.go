package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"govard/internal/conventions"

	"gopkg.in/yaml.v3"
)

func NormalizeConfig(config *Config, root string) {
	if config == nil {
		return
	}

	normalizeBlueprintRegistryConfig(&config.BlueprintRegistry)
	config.Framework = NormalizeFrameworkAlias(config.Framework)
	normalizeAuditConfig(&config.Audit)
	// Frameworks without a lint profile (e.g. symfony, laravel) must not
	// carry a default audit.lint.provider that promises a non-existent gate.
	// Clearing it here prevents govard init/bootstrap from writing
	// audit.lint.provider: govard for those frameworks.
	if !FrameworkSupportsAuditLint(config.Framework) {
		config.Audit.Lint.Provider = ""
	}
	config.StoreDomains = normalizeStoreDomainMappings(config.StoreDomains)
	config.TablePrefix = NormalizeTablePrefix(config.TablePrefix)
	if config.TablePrefix == "" && root != "" {
		config.TablePrefix = DetectFrameworkTablePrefix(root, config.Framework)
	}

	fwConfig, ok := GetFrameworkConfig(config.Framework)
	profileResult, profileErr := ResolveRuntimeProfile(config.Framework, config.FrameworkVersion)
	profileAvailable := profileErr == nil
	profile := profileResult.Profile

	if config.Stack.WebRoot == "" || (config.Stack.WebRoot == "/" && root != "") {
		detected := DetectWebRoot(root, config.Framework)
		if detected != "" {
			config.Stack.WebRoot = detected
		} else if config.Stack.WebRoot == "" {
			if profileAvailable && profile.WebRoot != "" {
				config.Stack.WebRoot = profile.WebRoot
			} else if ok && fwConfig.NGINXPUBLIC != "" {
				config.Stack.WebRoot = fwConfig.NGINXPUBLIC
			}
		}
	}

	// If db is not declared, treat as "none" (e.g. frontend-only or external DB projects).
	// Callers must explicitly declare the DB service they want.
	if config.Stack.Services.DB == "" {
		config.Stack.Services.DB = "none"
	}

	config.Stack.Services.DB = strings.ToLower(config.Stack.Services.DB)

	if config.Stack.DBVersion == "" {
		if config.Stack.Services.DB == "none" {
			config.Stack.DBVersion = ""
		} else if profileAvailable &&
			strings.EqualFold(config.Stack.Services.DB, profile.DB) &&
			profile.DBVersion != "" {
			config.Stack.DBVersion = profile.DBVersion
		} else if config.Stack.Services.DB == "mysql" && ok && fwConfig.DefaultMySQLVer != "" {
			config.Stack.DBVersion = fwConfig.DefaultMySQLVer
		} else if config.Stack.Services.DB == "mysql" {
			config.Stack.DBVersion = "8.4"
		} else if ok && fwConfig.DefaultDBVer != "" {
			config.Stack.DBVersion = fwConfig.DefaultDBVer
		} else {
			config.Stack.DBVersion = "10.6"
		}
	}

	if config.Stack.PHPVersion == "" {
		if profileAvailable && profile.PHPVersion != "" {
			config.Stack.PHPVersion = profile.PHPVersion
		} else if ok && fwConfig.DefaultPHP != "" {
			config.Stack.PHPVersion = fwConfig.DefaultPHP
		}
		// If DefaultPHP is empty (e.g., custom framework with no PHP),
		// leave PHPVersion empty to indicate no PHP container is needed
	}

	if config.Stack.PythonVersion == "" {
		if ok && fwConfig.DefaultPythonVer != "" {
			config.Stack.PythonVersion = fwConfig.DefaultPythonVer
		}
		// If DefaultPythonVer is empty (e.g., a non-Python framework), leave
		// PythonVersion empty - no Python container is needed.
	}

	if config.Stack.NodeVersion == "" {
		if profileAvailable && profile.NodeVersion != "" {
			config.Stack.NodeVersion = profile.NodeVersion
		} else if ok && fwConfig.DefaultNodeVer != "" {
			config.Stack.NodeVersion = fwConfig.DefaultNodeVer
		} else {
			config.Stack.NodeVersion = "24"
		}
	}

	if config.Stack.XdebugSession == "" {
		if profileAvailable && profile.XdebugSession != "" {
			config.Stack.XdebugSession = profile.XdebugSession
		} else {
			config.Stack.XdebugSession = "PHPSTORM"
		}
	}

	if config.Stack.WebRoot == "" {
		if profileAvailable && profile.WebRoot != "" {
			config.Stack.WebRoot = profile.WebRoot
		} else if ok && fwConfig.NGINXPUBLIC != "" {
			config.Stack.WebRoot = fwConfig.NGINXPUBLIC
		}
	}

	if config.Stack.NginxVersion == "" {
		if profileAvailable && profile.NginxVersion != "" {
			config.Stack.NginxVersion = profile.NginxVersion
		} else if ok && fwConfig.DefaultNginxVer != "" {
			config.Stack.NginxVersion = fwConfig.DefaultNginxVer
		} else {
			config.Stack.NginxVersion = "1.28"
		}
	}

	if config.Stack.ComposerVersion == "" {
		if profileAvailable && profile.ComposerVersion != "" {
			config.Stack.ComposerVersion = profile.ComposerVersion
		} else if ok && fwConfig.DefaultComposerVer != "" {
			config.Stack.ComposerVersion = fwConfig.DefaultComposerVer
		}
		// If DefaultComposerVer is empty (e.g., custom framework with no PHP),
		// leave ComposerVersion empty
	}

	// Safety override: Composer 2.3+ requires PHP >= 7.2.5.
	// Force Composer 2.2 LTS when running on older PHP, even if "latest" was requested.
	if config.Stack.ComposerVersion == "latest" &&
		config.Stack.PHPVersion != "" &&
		!IsNumericDotVersionAtLeast(config.Stack.PHPVersion, "7.2.5") {
		config.Stack.ComposerVersion = "2.2"
	}

	if config.Stack.ApacheVersion == "" {
		if profileAvailable && profile.ApacheVersion != "" {
			config.Stack.ApacheVersion = profile.ApacheVersion
		} else if ok && fwConfig.DefaultApacheVer != "" {
			config.Stack.ApacheVersion = fwConfig.DefaultApacheVer
		} else {
			config.Stack.ApacheVersion = "2.4"
		}
	}

	if config.Stack.Services.WebServer == "" {
		if config.Stack.WebServer != "" {
			config.Stack.Services.WebServer = config.Stack.WebServer
		} else if profileAvailable && profile.WebServer != "" {
			config.Stack.Services.WebServer = profile.WebServer
		} else if ok && fwConfig.DefaultWebServer != "" {
			config.Stack.Services.WebServer = fwConfig.DefaultWebServer
		} else {
			config.Stack.Services.WebServer = "nginx"
		}
	}

	config.Stack.Services.WebServer = strings.ToLower(config.Stack.Services.WebServer)

	// If search/cache/queue are not declared, treat as "none".
	// Callers must explicitly set the service they want — we do not auto-fill from profile.
	if config.Stack.Services.Search == "" {
		config.Stack.Services.Search = "none"
	}
	config.Stack.Services.Search = strings.ToLower(config.Stack.Services.Search)

	if config.Stack.Services.Cache == "" {
		config.Stack.Services.Cache = "none"
	}
	config.Stack.Services.Cache = strings.ToLower(config.Stack.Services.Cache)

	if config.Stack.Services.Queue == "" {
		config.Stack.Services.Queue = "none"
	}
	config.Stack.Services.Queue = strings.ToLower(config.Stack.Services.Queue)

	// Sync Features and Services (Service Presence as Master)
	// 1. If service string is missing or "none", ensure feature is false.
	// 2. If feature is true but service is "none", service wins (it's disabled).

	config.Stack.Features.Cache = config.Stack.Services.Cache != "" && config.Stack.Services.Cache != "none"
	config.Stack.Features.Search = config.Stack.Services.Search != "" && config.Stack.Services.Search != "none"
	config.Stack.Features.Queue = config.Stack.Services.Queue != "" && config.Stack.Services.Queue != "none"
	config.Stack.WebServer = config.Stack.Services.WebServer

	if config.Stack.Services.Cache == "none" {
		config.Stack.CacheVersion = ""
	} else if config.Stack.CacheVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Cache, profile.Cache) &&
		profile.CacheVersion != "" {
		config.Stack.CacheVersion = profile.CacheVersion
	} else if config.Stack.CacheVersion == "" {
		if config.Stack.Services.Cache == "valkey" {
			config.Stack.CacheVersion = "9.0"
		} else if ok && fwConfig.DefaultCacheVer != "" && strings.EqualFold(config.Stack.Services.Cache, fwConfig.DefaultCache) {
			config.Stack.CacheVersion = fwConfig.DefaultCacheVer
		} else {
			config.Stack.CacheVersion = "7.4"
		}
	}

	if config.Stack.Services.Search == "none" {
		config.Stack.SearchVersion = ""
	} else if config.Stack.SearchVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Search, profile.Search) &&
		profile.SearchVersion != "" {
		config.Stack.SearchVersion = profile.SearchVersion
	} else if config.Stack.SearchVersion == "" && ok && fwConfig.DefaultSearchVer != "" {
		config.Stack.SearchVersion = fwConfig.DefaultSearchVer
	} else if config.Stack.SearchVersion == "" {
		if config.Stack.Services.Search == "elasticsearch" {
			config.Stack.SearchVersion = "8.15"
		} else {
			config.Stack.SearchVersion = "3.0"
		}
	}

	if !config.Stack.Features.Varnish {
		config.Stack.VarnishVersion = ""
	} else if config.Stack.VarnishVersion == "" &&
		profileAvailable &&
		profile.VarnishVersion != "" {
		config.Stack.VarnishVersion = profile.VarnishVersion
	} else if config.Stack.VarnishVersion == "" && ok && fwConfig.DefaultVarnishVer != "" {
		config.Stack.VarnishVersion = fwConfig.DefaultVarnishVer
	} else if config.Stack.VarnishVersion == "" {
		config.Stack.VarnishVersion = "8.0"
	}

	if config.Stack.Services.Queue == "none" {
		config.Stack.QueueVersion = ""
	} else if config.Stack.QueueVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Queue, profile.Queue) &&
		profile.QueueVersion != "" {
		config.Stack.QueueVersion = profile.QueueVersion
	} else if config.Stack.QueueVersion == "" && ok && fwConfig.DefaultQueueVer != "" {
		config.Stack.QueueVersion = fwConfig.DefaultQueueVer
	} else if config.Stack.QueueVersion == "" {
		config.Stack.QueueVersion = "4.2"
	}

	if config.Stack.UserID == 0 {
		uid := os.Getuid()
		if uid < 0 {
			uid = 1000
		}
		config.Stack.UserID = uid
	}
	if config.Stack.GroupID == 0 {
		gid := os.Getgid()
		if gid < 0 {
			gid = 1000
		}
		config.Stack.GroupID = gid
	}

	if len(config.Stack.ChownDirList) == 0 {
		config.Stack.ChownDirList = GetDefaultChownDirList(config.Framework)
	}

	if config.Stack.WebRoot != "" && !strings.HasPrefix(config.Stack.WebRoot, "/") {
		config.Stack.WebRoot = "/" + config.Stack.WebRoot
	}

	if config.Remotes != nil {
		for name, remote := range config.Remotes {
			if remote.Port == 0 {
				remote.Port = 22
			}
			remote.Auth.Method = NormalizeRemoteAuthMethod(remote.Auth.Method)
			remote.Auth.KeyPath = strings.TrimSpace(remote.Auth.KeyPath)
			remote.Auth.KnownHostsFile = strings.TrimSpace(remote.Auth.KnownHostsFile)
			remote.Paths.Media = strings.TrimSpace(remote.Paths.Media)
			if remote.Auth.KnownHostsFile != "" {
				remote.Auth.StrictHostKey = true
			}
			config.Remotes[name] = remote
		}
	}
}

func normalizeAuditConfig(config *AuditConfig) {
	if config == nil {
		return
	}
	config.Lint.Provider = NormalizeProviderName(config.Lint.Provider)
	if config.Lint.Provider == "" {
		config.Lint.Provider = "govard"
	}
	if config.Lint.ExternalProviders == nil {
		return
	}
	normalized := make(map[string]ExternalLintProviderConfig, len(config.Lint.ExternalProviders))
	ids := make([]string, 0, len(config.Lint.ExternalProviders))
	for id := range config.Lint.ExternalProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		provider := config.Lint.ExternalProviders[id]
		provider.Type = NormalizeProviderName(provider.Type)
		normalizedID := NormalizeProviderName(id)
		if _, exists := normalized[normalizedID]; exists {
			if config.Lint.externalProviderKeyCollision == "" {
				config.Lint.externalProviderKeyCollision = normalizedID
			}
			continue
		}
		normalized[normalizedID] = provider
	}
	config.Lint.ExternalProviders = normalized
}

// CollectConfigDrift returns human-readable drift warnings comparing the
// persisted config against the detected framework version and its profile.
// Used by govard doctor to surface yml drift.
func CollectConfigDrift(cfg Config, meta ProjectMetadata) []string {
	warnings := []string{}
	desiredVersion := strings.TrimSpace(meta.Version)
	if desiredVersion == "" {
		desiredVersion = strings.TrimSpace(cfg.FrameworkVersion)
	}
	if desiredVersion != "" && strings.TrimSpace(cfg.FrameworkVersion) != desiredVersion {
		warnings = append(warnings, fmt.Sprintf("framework_version %s→%s", strings.TrimSpace(cfg.FrameworkVersion), desiredVersion))
	}
	profileResult, err := ResolveRuntimeProfile(cfg.Framework, desiredVersion)
	if err != nil {
		return warnings
	}
	p := profileResult.Profile
	if p.PHPVersion != "" && strings.TrimSpace(cfg.Stack.PHPVersion) != p.PHPVersion {
		warnings = append(warnings, fmt.Sprintf("stack.php_version %s→%s", strings.TrimSpace(cfg.Stack.PHPVersion), p.PHPVersion))
	}
	if p.DBVersion != "" && strings.TrimSpace(cfg.Stack.DBVersion) != p.DBVersion {
		// Only warn if DB service is not "none"
		if strings.ToLower(strings.TrimSpace(cfg.Stack.Services.DB)) != "none" && cfg.Stack.Services.DB != "" {
			warnings = append(warnings, fmt.Sprintf("stack.db_version %s→%s", strings.TrimSpace(cfg.Stack.DBVersion), p.DBVersion))
		}
	}
	if p.NodeVersion != "" && strings.TrimSpace(cfg.Stack.NodeVersion) != p.NodeVersion {
		warnings = append(warnings, fmt.Sprintf("stack.node_version %s→%s", strings.TrimSpace(cfg.Stack.NodeVersion), p.NodeVersion))
	}
	if p.SearchVersion != "" && strings.TrimSpace(cfg.Stack.SearchVersion) != p.SearchVersion {
		if strings.ToLower(strings.TrimSpace(cfg.Stack.Services.Search)) != "none" && cfg.Stack.Services.Search != "" {
			warnings = append(warnings, fmt.Sprintf("stack.search_version %s→%s", strings.TrimSpace(cfg.Stack.SearchVersion), p.SearchVersion))
		}
	}
	// cache_version drift (only when a cache service is active).
	if p.CacheVersion != "" && strings.TrimSpace(cfg.Stack.CacheVersion) != p.CacheVersion {
		if strings.ToLower(strings.TrimSpace(cfg.Stack.Services.Cache)) != "none" && cfg.Stack.Services.Cache != "" {
			warnings = append(warnings, fmt.Sprintf("stack.cache_version %s→%s", strings.TrimSpace(cfg.Stack.CacheVersion), p.CacheVersion))
		}
	}
	// services.search (the search backend service name) drift.
	if p.Search != "" && strings.TrimSpace(cfg.Stack.Services.Search) != strings.TrimSpace(p.Search) {
		if strings.ToLower(strings.TrimSpace(cfg.Stack.Services.Search)) != "none" && cfg.Stack.Services.Search != "" {
			warnings = append(warnings, fmt.Sprintf("stack.services.search %s→%s", strings.TrimSpace(cfg.Stack.Services.Search), p.Search))
		}
	}
	// Node 14 EOL hygiene: warn when config still pins EOL Node 14 (recommend 18+).
	// Also surface profile-driven EOL warning if profile itself is EOL (e.g. future defaults).
	// Both checks are unconditional and deduped by value equality.
	cfgNode := strings.TrimSpace(cfg.Stack.NodeVersion)
	profileNode := strings.TrimSpace(profileResult.Profile.NodeVersion)
	if IsNodeVersionEOL(cfgNode) {
		warnings = append(warnings, NodeEOLWarning(cfgNode))
	}
	if IsNodeVersionEOL(profileNode) {
		msg := NodeEOLWarning(profileNode)
		already := false
		for _, existing := range warnings {
			if existing == msg {
				already = true
				break
			}
		}
		if !already {
			warnings = append(warnings, msg)
		}
	}
	return warnings
}

// CollectConfigDriftWarningsForTest loads the raw config from dir and reports drift vs detected metadata.
func CollectConfigDriftWarningsForTest(dir string) []string {
	cfg, err := LoadRawConfigFromDir(dir, false)
	if err != nil {
		return nil
	}
	meta := DetectFramework(dir)
	return CollectConfigDrift(cfg, meta)
}

// CollectConfigDriftWarningsForTestWithConfig reports drift for an in-memory config and metadata pair.
func CollectConfigDriftWarningsForTestWithConfig(cfg Config, meta ProjectMetadata) []string {
	return CollectConfigDrift(cfg, meta)
}

// SyncConfigDriftForTest yq-updates .govard.yml to align framework_version and stack versions with the
// detected profile. When dryRun is true it returns the planned changes without writing.
// When commit is true it also runs git add/commit if the directory is a git repo.
func SyncConfigDriftForTest(dir string, dryRun bool, commit bool) ([]string, error) {
	rawCfg, err := LoadRawConfigFromDir(dir, false)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	meta := DetectFramework(dir)
	desiredVersion := strings.TrimSpace(meta.Version)
	if desiredVersion == "" {
		desiredVersion = strings.TrimSpace(rawCfg.FrameworkVersion)
	}
	// Resolve profile for desired version; fall back to current framework_version if needed.
	profileResult, err := ResolveRuntimeProfile(rawCfg.Framework, desiredVersion)
	if err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}
	p := profileResult.Profile

	changes := []string{}
	updated := rawCfg

	// framework_version
	if desiredVersion != "" && strings.TrimSpace(updated.FrameworkVersion) != desiredVersion {
		changes = append(changes, fmt.Sprintf("framework_version %s→%s", strings.TrimSpace(updated.FrameworkVersion), desiredVersion))
		updated.FrameworkVersion = desiredVersion
	}
	// php_version
	if p.PHPVersion != "" && strings.TrimSpace(updated.Stack.PHPVersion) != p.PHPVersion {
		changes = append(changes, fmt.Sprintf("stack.php_version %s→%s", strings.TrimSpace(updated.Stack.PHPVersion), p.PHPVersion))
		updated.Stack.PHPVersion = p.PHPVersion
	}
	// db_version (only if DB service is active)
	if p.DBVersion != "" && strings.ToLower(strings.TrimSpace(updated.Stack.Services.DB)) != "none" && updated.Stack.Services.DB != "" {
		if strings.TrimSpace(updated.Stack.DBVersion) != p.DBVersion {
			changes = append(changes, fmt.Sprintf("stack.db_version %s→%s", strings.TrimSpace(updated.Stack.DBVersion), p.DBVersion))
			updated.Stack.DBVersion = p.DBVersion
		}
	}
	// node_version
	if p.NodeVersion != "" && strings.TrimSpace(updated.Stack.NodeVersion) != p.NodeVersion {
		changes = append(changes, fmt.Sprintf("stack.node_version %s→%s", strings.TrimSpace(updated.Stack.NodeVersion), p.NodeVersion))
		updated.Stack.NodeVersion = p.NodeVersion
	}
	// search_version (only if search service is active)
	if p.SearchVersion != "" && strings.ToLower(strings.TrimSpace(updated.Stack.Services.Search)) != "none" && updated.Stack.Services.Search != "" {
		if strings.TrimSpace(updated.Stack.SearchVersion) != p.SearchVersion {
			changes = append(changes, fmt.Sprintf("stack.search_version %s→%s", strings.TrimSpace(updated.Stack.SearchVersion), p.SearchVersion))
			updated.Stack.SearchVersion = p.SearchVersion
		}
	}
	// cache_version (only if cache service is active)
	if p.CacheVersion != "" && strings.ToLower(strings.TrimSpace(updated.Stack.Services.Cache)) != "none" && updated.Stack.Services.Cache != "" {
		if strings.TrimSpace(updated.Stack.CacheVersion) != p.CacheVersion {
			changes = append(changes, fmt.Sprintf("stack.cache_version %s→%s", strings.TrimSpace(updated.Stack.CacheVersion), p.CacheVersion))
			updated.Stack.CacheVersion = p.CacheVersion
		}
	}
	// services.search: sync the search backend service name (e.g. elasticsearch -> opensearch)
	if p.Search != "" && strings.ToLower(strings.TrimSpace(updated.Stack.Services.Search)) != "none" && updated.Stack.Services.Search != "" {
		if strings.TrimSpace(updated.Stack.Services.Search) != strings.TrimSpace(p.Search) {
			changes = append(changes, fmt.Sprintf("stack.services.search %s→%s", strings.TrimSpace(updated.Stack.Services.Search), p.Search))
			updated.Stack.Services.Search = p.Search
		}
	}

	if len(changes) == 0 {
		return nil, nil
	}
	if dryRun {
		return changes, nil
	}

	// Marshal via PrepareConfigForWrite to keep minimal YAML, then restore drift-synced values that
	// PrepareConfigForWrite might strip if they match defaults (we want explicit sync, so keep them).
	writable := PrepareConfigForWrite(updated)
	// Re-apply drifted values explicitly after PrepareConfigForWrite stripping.
	if updated.FrameworkVersion != "" {
		writable.FrameworkVersion = updated.FrameworkVersion
	}
	if updated.Stack.PHPVersion != "" {
		writable.Stack.PHPVersion = updated.Stack.PHPVersion
	}
	if updated.Stack.DBVersion != "" {
		writable.Stack.DBVersion = updated.Stack.DBVersion
	}
	if updated.Stack.NodeVersion != "" {
		writable.Stack.NodeVersion = updated.Stack.NodeVersion
	}
	if updated.Stack.SearchVersion != "" {
		writable.Stack.SearchVersion = updated.Stack.SearchVersion
	}
	if updated.Stack.CacheVersion != "" {
		writable.Stack.CacheVersion = updated.Stack.CacheVersion
	}
	if updated.Stack.Services.Search != "" {
		writable.Stack.Services.Search = updated.Stack.Services.Search
	}
	data, err := yaml.Marshal(&writable)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	path := filepath.Join(dir, conventions.BaseConfigFile)
	if err := os.WriteFile(path, data, conventions.DefaultFilePerm); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	if commit {
		// Best-effort git commit if dir is inside a git repo.
		_ = exec.Command("git", "-C", dir, "add", conventions.BaseConfigFile).Run()
		msg := fmt.Sprintf("chore(govard): sync %s drift (%s)", conventions.BaseConfigFile, strings.Join(changes, ", "))
		_ = exec.Command("git", "-C", dir, "commit", "-m", msg).Run()
	}
	return changes, nil
}
