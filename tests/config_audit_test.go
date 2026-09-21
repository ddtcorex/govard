package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
	_ "govard/internal/frameworks" // register framework capabilities (AuditLint)

	"gopkg.in/yaml.v3"
)

func TestAuditProviderConfigAcceptsExplicitDockerProvider(t *testing.T) {
	var config engine.Config
	if err := yaml.Unmarshal([]byte(`
project_name: audit-shop
domain: audit-shop.test
framework: magento2
audit:
  lint:
    provider: govard
    external_providers:
      team-ci:
        type: docker
        image: registry.example.com/team/glint:v3
        command: ["/usr/local/bin/glint", "--report-json", "/output/report.json"]
`), &config); err != nil {
		t.Fatal(err)
	}
	engine.NormalizeConfig(&config, "")
	if err := engine.ValidateConfig(config); err != nil {
		t.Fatal(err)
	}
	provider := config.Audit.Lint.ExternalProviders["team-ci"]
	if config.Audit.Lint.Provider != "govard" || provider.Type != "docker" {
		t.Fatalf("normalized provider config = %#v", config.Audit.Lint)
	}
	if provider.Image != "registry.example.com/team/glint:v3" || len(provider.Command) != 3 {
		t.Fatalf("provider payload changed during normalization: %#v", provider)
	}
}

func TestAuditProviderConfigDefaultsToGovardAndNormalizesNamesAndTypesOnly(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
		Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{ExternalProviders: map[string]engine.ExternalLintProviderConfig{
			" Team_CI ": {Type: " DOCKER ", Image: " registry.example.com/team/glint:v3 ", Command: []string{" /tool ", " --flag "}},
		}}},
	}
	engine.NormalizeConfig(&config, "")
	if config.Audit.Lint.Provider != "govard" {
		t.Fatalf("provider = %q, want govard", config.Audit.Lint.Provider)
	}
	provider, ok := config.Audit.Lint.ExternalProviders["team_ci"]
	if !ok || provider.Type != "docker" {
		t.Fatalf("normalized external providers = %#v", config.Audit.Lint.ExternalProviders)
	}
	if provider.Image != " registry.example.com/team/glint:v3 " || provider.Command[0] != " /tool " || provider.Command[1] != " --flag " {
		t.Fatalf("normalization changed provider payload: %#v", provider)
	}
}

func TestAuditProviderConfigRejectsInvalidValues(t *testing.T) {
	base := func() engine.Config {
		return engine.Config{
			ProjectName: "audit-shop",
			Domain:      "audit-shop.test",
			Framework:   "magento2",
			Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{ExternalProviders: map[string]engine.ExternalLintProviderConfig{
				"team-ci": {Type: "docker", Image: "registry.example.com/team/glint:v3", Command: []string{"/tool", "--report-json", "/output/report.json"}},
			}}},
		}
	}
	for name, mutate := range map[string]func(*engine.Config){
		"invalid provider identifier": func(config *engine.Config) {
			config.Audit.Lint.ExternalProviders = map[string]engine.ExternalLintProviderConfig{"team ci": config.Audit.Lint.ExternalProviders["team-ci"]}
		},
		"invalid type": func(config *engine.Config) {
			provider := config.Audit.Lint.ExternalProviders["team-ci"]
			provider.Type = "shell"
			config.Audit.Lint.ExternalProviders["team-ci"] = provider
		},
		"missing image": func(config *engine.Config) {
			provider := config.Audit.Lint.ExternalProviders["team-ci"]
			provider.Image = ""
			config.Audit.Lint.ExternalProviders["team-ci"] = provider
		},
		"empty command": func(config *engine.Config) {
			provider := config.Audit.Lint.ExternalProviders["team-ci"]
			provider.Command = nil
			config.Audit.Lint.ExternalProviders["team-ci"] = provider
		},
		"empty command argument": func(config *engine.Config) {
			provider := config.Audit.Lint.ExternalProviders["team-ci"]
			provider.Command[1] = "  "
			config.Audit.Lint.ExternalProviders["team-ci"] = provider
		},
		"unknown selected provider": func(config *engine.Config) { config.Audit.Lint.Provider = "missing" },
	} {
		t.Run(name, func(t *testing.T) {
			config := base()
			mutate(&config)
			engine.NormalizeConfig(&config, "")
			if err := engine.ValidateConfig(config); err == nil || !strings.Contains(err.Error(), "audit.lint") {
				t.Fatalf("error = %v, want audit lint validation error", err)
			}
		})
	}
}

func TestAuditProviderConfigRejectsNormalizedExternalProviderKeyCollision(t *testing.T) {
	var config engine.Config
	if err := yaml.Unmarshal([]byte(`
project_name: audit-shop
domain: audit-shop.test
framework: magento2
audit:
  lint:
    external_providers:
      team-ci:
        type: docker
        image: registry.example.com/team/first:v3
        command: ["/tool", "--report-json", "/output/report.json"]
      " TEAM-CI ":
        type: docker
        image: registry.example.com/team/second:v3
        command: ["/tool", "--report-json", "/output/report.json"]
`), &config); err != nil {
		t.Fatal(err)
	}
	engine.NormalizeConfig(&config, "")
	err := engine.ValidateConfig(config)
	if err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("error = %v, want normalized key collision", err)
	}
}

func TestPrepareConfigForWriteOmitsDefaultAuditLintProvider(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
	}
	engine.NormalizeConfig(&config, "")
	if config.Audit.Lint.Provider != "govard" {
		t.Fatalf("normalized provider = %q, want govard", config.Audit.Lint.Provider)
	}
	writable := engine.PrepareConfigForWrite(config)
	if writable.Audit.Lint.Provider != "" || writable.Audit.Lint.ExternalProviders != nil {
		t.Fatalf("writable audit = %#v, want empty", writable.Audit)
	}
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "audit:") {
		t.Fatalf("marshalled config still contains audit block:\n%s", data)
	}
}

func TestPrepareConfigForWriteKeepsExternalProvidersWithoutDefaultProvider(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
		Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{
			Provider: "govard",
			ExternalProviders: map[string]engine.ExternalLintProviderConfig{
				"team-ci": {Type: "docker", Image: "registry.example.com/team/glint:v3", Command: []string{"/tool", "--report-json", "/output/report.json"}},
			},
		}},
	}
	writable := engine.PrepareConfigForWrite(config)
	if writable.Audit.Lint.Provider != "" {
		t.Fatalf("writable provider = %q, want empty (default implied)", writable.Audit.Lint.Provider)
	}
	if _, ok := writable.Audit.Lint.ExternalProviders["team-ci"]; !ok {
		t.Fatalf("external providers lost: %#v", writable.Audit.Lint.ExternalProviders)
	}
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	lint, ok := raw["audit"].(map[string]any)["lint"].(map[string]any)
	if !ok {
		t.Fatalf("marshalled config lost audit.lint block:\n%s", data)
	}
	if _, ok := lint["provider"]; ok {
		t.Fatalf("marshalled config still contains default provider:\n%s", data)
	}
	if _, ok := lint["external_providers"]; !ok {
		t.Fatalf("marshalled config lost external_providers:\n%s", data)
	}
	// Round trip: the written YAML must reload with the govard default.
	var reloaded engine.Config
	if err := yaml.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	engine.NormalizeConfig(&reloaded, "")
	if reloaded.Audit.Lint.Provider != "govard" {
		t.Fatalf("reloaded provider = %q, want govard", reloaded.Audit.Lint.Provider)
	}
	if err := engine.ValidateConfig(reloaded); err != nil {
		t.Fatalf("reloaded config failed validation: %v", err)
	}
}

func TestPrepareConfigForWriteCollapsesEmptyExternalProvidersMap(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
		Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{
			Provider:          "govard",
			ExternalProviders: map[string]engine.ExternalLintProviderConfig{},
		}},
	}
	writable := engine.PrepareConfigForWrite(config)
	if writable.Audit.Lint.Provider != "" || writable.Audit.Lint.ExternalProviders != nil {
		t.Fatalf("writable audit = %#v, want empty", writable.Audit)
	}
}

func TestPrepareConfigForWriteKeepsCustomAuditLintProvider(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
		Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{
			Provider: "team-ci",
			ExternalProviders: map[string]engine.ExternalLintProviderConfig{
				"team-ci": {Type: "docker", Image: "registry.example.com/team/glint:v3", Command: []string{"/tool", "--report-json", "/output/report.json"}},
			},
		}},
	}
	writable := engine.PrepareConfigForWrite(config)
	if writable.Audit.Lint.Provider != "team-ci" {
		t.Fatalf("writable provider = %q, want team-ci", writable.Audit.Lint.Provider)
	}
}
