package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"

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
        image: registry.example.com/team/magelint:v3
        command: ["/usr/local/bin/magelint", "--report-json", "/output/report.json"]
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
	if provider.Image != "registry.example.com/team/magelint:v3" || len(provider.Command) != 3 {
		t.Fatalf("provider payload changed during normalization: %#v", provider)
	}
}

func TestAuditProviderConfigDefaultsToGovardAndNormalizesNamesAndTypesOnly(t *testing.T) {
	config := engine.Config{
		ProjectName: "audit-shop",
		Domain:      "audit-shop.test",
		Framework:   "magento2",
		Audit: engine.AuditConfig{Lint: engine.AuditLintConfig{ExternalProviders: map[string]engine.ExternalLintProviderConfig{
			" Team_CI ": {Type: " DOCKER ", Image: " registry.example.com/team/magelint:v3 ", Command: []string{" /tool ", " --flag "}},
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
	if provider.Image != " registry.example.com/team/magelint:v3 " || provider.Command[0] != " /tool " || provider.Command[1] != " --flag " {
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
				"team-ci": {Type: "docker", Image: "registry.example.com/team/magelint:v3", Command: []string{"/tool", "--report-json", "/output/report.json"}},
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
