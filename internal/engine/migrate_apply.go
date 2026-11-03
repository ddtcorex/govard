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
func ApplyMigration(cfg Config, migrated MigrationResult) Config {
	if migrated.ProjectName != "" {
		cfg.ProjectName = migrated.ProjectName
	}
	if migrated.Framework != "" {
		cfg.Framework = migrated.Framework
	}
	if migrated.PHPVersion != "" {
		cfg.Stack.PHPVersion = migrated.PHPVersion
	}
	if migrated.NodeVersion != "" {
		cfg.Stack.NodeVersion = migrated.NodeVersion
	}
	if migrated.ComposerVersion != "" {
		cfg.Stack.ComposerVersion = migrated.ComposerVersion
	}
	if migrated.WebServer != "" {
		cfg.Stack.Services.WebServer = migrated.WebServer
	}
	if migrated.DB != "" {
		cfg.Stack.Services.DB = migrated.DB
	}
	if migrated.DBVersion != "" {
		cfg.Stack.DBVersion = migrated.DBVersion
	}
	if migrated.WebRoot != "" {
		cfg.Stack.WebRoot = migrated.WebRoot
	}
	if migrated.TablePrefix != "" && FrameworkSupportsTablePrefix(cfg.Framework) {
		cfg.TablePrefix = migrated.TablePrefix
	}
	if migrated.XdebugEnabled {
		cfg.Stack.Features.Xdebug = true
	}

	// Services, the way DDEV taught us to read them.
	if migrated.SearchService != "" {
		cfg.Stack.Services.Search = migrated.SearchService
		cfg.Stack.SearchVersion = migrated.SearchVersion
		cfg.Stack.Features.Search = migrated.SearchService != "none"
	}
	if migrated.CacheService != "" {
		cfg.Stack.Services.Cache = migrated.CacheService
		cfg.Stack.CacheVersion = migrated.CacheVersion
		cfg.Stack.Features.Cache = migrated.CacheService != "none"
	}
	if migrated.QueueService != "" {
		cfg.Stack.Services.Queue = migrated.QueueService
		cfg.Stack.QueueVersion = migrated.QueueVersion
		cfg.Stack.Features.Queue = migrated.QueueService != "none"
	}

	// Varnish is only touched when the source actually said something about it.
	if migrated.VarnishSpecified {
		cfg.Stack.Features.Varnish = migrated.VarnishEnabled
		if migrated.VarnishVersion != "" {
			cfg.Stack.VarnishVersion = migrated.VarnishVersion
		}
	}

	for _, domain := range migrated.ExtraDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" || containsString(cfg.ExtraDomains, domain) {
			continue
		}
		cfg.ExtraDomains = append(cfg.ExtraDomains, domain)
	}

	if len(migrated.Remotes) > 0 {
		cfg.Remotes = migrated.Remotes
	}

	return cfg
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// MigrationSummary renders what actually happened, read from the configuration
// that will be written rather than from the intent at merge time. A value the
// source declared but the final config does not carry is reported as kept, not
// as mapped, because an existing .govard.yml or a framework profile pin may have
// won the merge.
func MigrationSummary(migrated MigrationResult, cfg Config) []string {
	var lines []string

	type field struct {
		label    string
		source   string
		final    string
		willKeep bool
	}
	fields := []field{
		{"name", migrated.ProjectName, cfg.ProjectName, false},
		{"type", migrated.Framework, cfg.Framework, false},
		{"php_version", migrated.PHPVersion, cfg.Stack.PHPVersion, false},
		{"nodejs_version", migrated.NodeVersion, cfg.Stack.NodeVersion, false},
		{"composer_version", migrated.ComposerVersion, cfg.Stack.ComposerVersion, false},
		{"webserver_type", migrated.WebServer, cfg.Stack.Services.WebServer, false},
		{"database.type", migrated.DB, cfg.Stack.Services.DB, false},
		{"database.version", migrated.DBVersion, cfg.Stack.DBVersion, false},
		{"docroot", migrated.WebRoot, cfg.Stack.WebRoot, false},
		{"xdebug_enabled", boolValue(migrated.XdebugEnabled), boolValue(cfg.Stack.Features.Xdebug), false},
	}
	for _, f := range fields {
		switch {
		case f.source != "" && f.source == f.final:
			lines = append(lines, fmt.Sprintf("mapped %s: %s", f.label, f.final))
		case f.source != "" && f.source != f.final:
			lines = append(lines, fmt.Sprintf("kept %s: %s (the source had %s)", f.label, f.final, f.source))
		}
	}

	services := []struct {
		label  string
		source string
		final  string
	}{
		{"search", migrated.SearchService, cfg.Stack.Services.Search},
		{"cache", migrated.CacheService, cfg.Stack.Services.Cache},
		{"queue", migrated.QueueService, cfg.Stack.Services.Queue},
	}
	for _, s := range services {
		if s.source != "" && s.source != s.final {
			lines = append(lines, fmt.Sprintf("kept %s: %s (the source had %s)", s.label, s.final, s.source))
			continue
		}
		if s.source != "" {
			lines = append(lines, fmt.Sprintf("mapped %s: %s", s.label, s.final))
			continue
		}
		lines = append(lines, fmt.Sprintf("defaulted %s: %s", s.label, s.final))
	}

	lines = append(lines, fmt.Sprintf("defaulted varnish: %t", cfg.Stack.Features.Varnish))

	for _, domain := range cfg.ExtraDomains {
		if containsString(migrated.ExtraDomains, domain) {
			lines = append(lines, fmt.Sprintf("mapped additional domain: %s", domain))
		}
	}
	for _, unmapped := range migrated.Unmapped {
		lines = append(lines, "not migrated "+unmapped)
	}

	return lines
}

func boolValue(value bool) string {
	if value {
		return "true"
	}

	return ""
}
