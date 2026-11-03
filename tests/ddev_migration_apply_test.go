package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
)

func magentoConfig() engine.Config {
	return engine.Config{
		Framework: "magento2",
		Stack: engine.Stack{
			Services: engine.Services{Search: "opensearch", Cache: "redis", Queue: "rabbitmq"},
			Features: engine.Features{Search: true, Cache: true, Queue: true, Varnish: true},
		},
	}
}

func TestApplyMigrationKeepsProfileDefaultsWhenDDEVSaysNothing(t *testing.T) {
	cfg := magentoConfig()

	got := engine.ApplyMigration(cfg, engine.MigrationResult{ProjectName: "acme-shop"})

	if got.Stack.Services.Search != "opensearch" || !got.Stack.Features.Search {
		t.Fatalf("search = %q (enabled %t), want the profile default kept", got.Stack.Services.Search, got.Stack.Features.Search)
	}
	if got.Stack.Services.Cache != "redis" || !got.Stack.Features.Cache {
		t.Fatalf("cache = %q (enabled %t), want the profile default kept", got.Stack.Services.Cache, got.Stack.Features.Cache)
	}
	if got.Stack.Services.Queue != "rabbitmq" {
		t.Fatalf("queue = %q, want the profile default kept", got.Stack.Services.Queue)
	}
	if !got.Stack.Features.Varnish {
		t.Fatalf("varnish = false, want the profile default kept when the source is silent")
	}

	summary := strings.Join(engine.MigrationSummary(engine.MigrationResult{ProjectName: "acme-shop"}, got), " | ")
	if !strings.Contains(summary, "defaulted search: opensearch") {
		t.Fatalf("summary = %q, want a defaulted search entry", summary)
	}
}

func TestApplyMigrationAppliesWhatTheSourceSays(t *testing.T) {
	cfg := magentoConfig()

	got := engine.ApplyMigration(cfg, engine.MigrationResult{
		SearchService:    "elasticsearch",
		SearchVersion:    "8.11",
		CacheService:     "none",
		VarnishSpecified: true,
		VarnishEnabled:   false,
	})

	if got.Stack.Services.Search != "elasticsearch" || got.Stack.SearchVersion != "8.11" {
		t.Fatalf("search = %q %q, want the migrated value", got.Stack.Services.Search, got.Stack.SearchVersion)
	}
	if got.Stack.Features.Cache || got.Stack.Services.Cache != "none" {
		t.Fatalf("cache = %q (enabled %t), want none as the source said", got.Stack.Services.Cache, got.Stack.Features.Cache)
	}
	if got.Stack.Features.Varnish {
		t.Fatalf("varnish = true, want it disabled because the source said so")
	}
	summary := strings.Join(engine.MigrationSummary(engine.MigrationResult{SearchService: "elasticsearch"}, got), " | ")
	if !strings.Contains(summary, "mapped search: elasticsearch") {
		t.Fatalf("summary = %q, want a mapped search entry", summary)
	}
}

func TestApplyMigrationCarriesDomainsAndToolchain(t *testing.T) {
	cfg := magentoConfig()

	got := engine.ApplyMigration(cfg, engine.MigrationResult{
		PHPVersion:    "8.3",
		NodeVersion:   "20",
		WebServer:     "apache",
		XdebugEnabled: true,
		ExtraDomains:  []string{"admin.acme-shop.test", "shop.local", "shop.local"},
	})

	if got.Stack.PHPVersion != "8.3" || got.Stack.NodeVersion != "20" {
		t.Fatalf("php/node = %q/%q, want 8.3/20", got.Stack.PHPVersion, got.Stack.NodeVersion)
	}
	if got.Stack.Services.WebServer != "apache" {
		t.Fatalf("web server = %q, want apache", got.Stack.Services.WebServer)
	}
	if !got.Stack.Features.Xdebug {
		t.Fatalf("xdebug = false, want true")
	}
	if len(got.ExtraDomains) != 2 {
		t.Fatalf("ExtraDomains = %v, want both domains with the duplicate dropped", got.ExtraDomains)
	}
	summary := strings.Join(engine.MigrationSummary(engine.MigrationResult{ExtraDomains: []string{"shop.local"}}, got), " | ")
	if !strings.Contains(summary, "mapped additional domain: shop.local") {
		t.Fatalf("summary = %q, want the extra domain named", summary)
	}
}

func TestApplyMigrationReportsWhatItCannotCarry(t *testing.T) {
	cfg := magentoConfig()

	got := engine.ApplyMigration(cfg, engine.MigrationResult{
		Unmapped: []string{"hooks: Govard hooks are declared in .govard.yml; re-add them there"},
	})

	summary := strings.Join(engine.MigrationSummary(engine.MigrationResult{
		Unmapped: []string{"hooks: Govard hooks are declared in .govard.yml; re-add them there"},
	}, got), " | ")
	if !strings.Contains(summary, "not migrated hooks:") {
		t.Fatalf("summary = %q, want the dropped key named", summary)
	}
}
