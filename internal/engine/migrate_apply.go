package engine

import (
	"fmt"
	"strings"
)

// ApplyMigration merges a migration result into a project configuration.
//
// The rule that matters: a source that does not mention a setting must leave the
// project's own default in place. DDEV, for instance, never mentions a search
// engine or Redis, and an earlier version of this code read that silence as
// "none", which left every migrated Magento project without a working
// storefront. The returned notes are what the init summary prints: every mapped
// key, every default that was kept, and every key that could not be mapped.
func ApplyMigration(cfg Config, migrated MigrationResult) (Config, []string) {
	var notes []string

	mapped := func(key, value string) {
		if value != "" {
			notes = append(notes, fmt.Sprintf("mapped %s: %s", key, value))
		}
	}
	kept := func(key, value string) {
		notes = append(notes, fmt.Sprintf("defaulted %s: %s", key, value))
	}

	if migrated.ProjectName != "" {
		cfg.ProjectName = migrated.ProjectName
		mapped("name", migrated.ProjectName)
	}
	if migrated.Framework != "" {
		cfg.Framework = migrated.Framework
		mapped("type", migrated.Framework)
	}
	if migrated.PHPVersion != "" {
		cfg.Stack.PHPVersion = migrated.PHPVersion
		mapped("php_version", migrated.PHPVersion)
	}
	if migrated.NodeVersion != "" {
		cfg.Stack.NodeVersion = migrated.NodeVersion
		mapped("nodejs_version", migrated.NodeVersion)
	}
	if migrated.ComposerVersion != "" {
		cfg.Stack.ComposerVersion = migrated.ComposerVersion
		mapped("composer_version", migrated.ComposerVersion)
	}
	if migrated.WebServer != "" {
		cfg.Stack.Services.WebServer = migrated.WebServer
		mapped("webserver_type", migrated.WebServer)
	}
	if migrated.DB != "" {
		cfg.Stack.Services.DB = migrated.DB
		mapped("database.type", migrated.DB)
	}
	if migrated.DBVersion != "" {
		cfg.Stack.DBVersion = migrated.DBVersion
		mapped("database.version", migrated.DBVersion)
	}
	if migrated.WebRoot != "" {
		cfg.Stack.WebRoot = migrated.WebRoot
		mapped("docroot", migrated.WebRoot)
	}
	if migrated.TablePrefix != "" && FrameworkSupportsTablePrefix(cfg.Framework) {
		cfg.TablePrefix = migrated.TablePrefix
		mapped("table_prefix", migrated.TablePrefix)
	}
	if migrated.XdebugEnabled {
		cfg.Stack.Features.Xdebug = true
		mapped("xdebug_enabled", "true")
	}

	// Services, the way DDEV taught us to read them.
	if migrated.SearchService != "" {
		cfg.Stack.Services.Search = migrated.SearchService
		cfg.Stack.SearchVersion = migrated.SearchVersion
		cfg.Stack.Features.Search = migrated.SearchService != "none"
		mapped("search", migrated.SearchService)
	} else {
		kept("search", cfg.Stack.Services.Search)
	}
	if migrated.CacheService != "" {
		cfg.Stack.Services.Cache = migrated.CacheService
		cfg.Stack.CacheVersion = migrated.CacheVersion
		cfg.Stack.Features.Cache = migrated.CacheService != "none"
		mapped("cache", migrated.CacheService)
	} else {
		kept("cache", cfg.Stack.Services.Cache)
	}
	if migrated.QueueService != "" {
		cfg.Stack.Services.Queue = migrated.QueueService
		cfg.Stack.QueueVersion = migrated.QueueVersion
		cfg.Stack.Features.Queue = migrated.QueueService != "none"
		mapped("queue", migrated.QueueService)
	} else {
		kept("queue", cfg.Stack.Services.Queue)
	}

	// Varnish is only touched when the source actually said something about it.
	if migrated.VarnishSpecified {
		cfg.Stack.Features.Varnish = migrated.VarnishEnabled
		if migrated.VarnishVersion != "" {
			cfg.Stack.VarnishVersion = migrated.VarnishVersion
		}
		notes = append(notes, fmt.Sprintf("mapped varnish: %t", migrated.VarnishEnabled))
	} else {
		kept("varnish", fmt.Sprintf("%t", cfg.Stack.Features.Varnish))
	}

	for _, domain := range migrated.ExtraDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" || containsString(cfg.ExtraDomains, domain) {
			continue
		}
		cfg.ExtraDomains = append(cfg.ExtraDomains, domain)
		notes = append(notes, fmt.Sprintf("mapped additional domain: %s", domain))
	}

	if len(migrated.Remotes) > 0 {
		cfg.Remotes = migrated.Remotes
		notes = append(notes, fmt.Sprintf("mapped remotes: %d", len(migrated.Remotes)))
	}

	for _, unmapped := range migrated.Unmapped {
		notes = append(notes, "not migrated "+unmapped)
	}

	return cfg, notes
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
