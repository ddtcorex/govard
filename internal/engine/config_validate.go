package engine

import (
	"fmt"
	"strings"
)

var (
	validWebServers = map[string]struct{}{
		"none":   {},
		"nginx":  {},
		"apache": {},
		"hybrid": {},
	}
	validSearchServices = map[string]struct{}{
		"none":          {},
		"opensearch":    {},
		"elasticsearch": {},
	}
	validCacheServices = map[string]struct{}{
		"none":   {},
		"redis":  {},
		"valkey": {},
	}
	validQueueServices = map[string]struct{}{
		"none":     {},
		"rabbitmq": {},
	}
)

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ProjectName) == "" {
		return fmt.Errorf("project_name is required")
	}
	if strings.TrimSpace(cfg.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.ContainsAny(cfg.Domain, " \t\r\n") {
		return fmt.Errorf("domain cannot contain whitespace")
	}
	if !ValidateTablePrefix(cfg.TablePrefix) {
		return fmt.Errorf("table_prefix %q is invalid (allowed: letters, numbers, and underscore)", cfg.TablePrefix)
	}
	if err := validateBlueprintRegistryConfig(cfg.BlueprintRegistry); err != nil {
		return err
	}
	// Audit lint is framework-owned. Frameworks without a lint profile
	// (e.g. symfony, laravel) may have an empty audit section; do not
	// default it to govard or require validation in that case. If they
	// explicitly configure a provider, still validate it.
	if FrameworkSupportsAuditLint(cfg.Framework) || strings.TrimSpace(cfg.Audit.Lint.Provider) != "" || len(cfg.Audit.Lint.ExternalProviders) > 0 {
		if err := validateAuditConfig(cfg.Audit); err != nil {
			return err
		}
	}
	if cfg.Stack.XdebugVersion != "" && !ValidateXdebugVersion(cfg.Stack.XdebugVersion) {
		return fmt.Errorf("stack.xdebug_version %q is invalid (use a PECL Xdebug version, e.g. 3.5.3)", cfg.Stack.XdebugVersion)
	}
	if cfg.Stack.Features.FrontendSync && !FrameworkSupportsFrontendSync(cfg.Framework) {
		return fmt.Errorf("stack.features.frontend_sync is not supported for framework %q", cfg.Framework)
	}

	for host, mapping := range cfg.StoreDomains {
		trimmedHost := strings.TrimSpace(host)
		if trimmedHost == "" {
			return fmt.Errorf("store_domains host cannot be empty")
		}
		if strings.ContainsAny(trimmedHost, " \t\r\n") {
			return fmt.Errorf("store_domains host '%s' cannot contain whitespace", host)
		}
		if strings.TrimSpace(mapping.Code) == "" {
			return fmt.Errorf("store_domains host '%s' is missing code", host)
		}
		switch mapping.ScopeType() {
		case "", "store", "website":
		default:
			return fmt.Errorf("store_domains host '%s' has unsupported type '%s' (allowed: store, website)", host, mapping.Type)
		}
	}

	if err := validateService("stack.services.web_server", cfg.Stack.Services.WebServer, validWebServers); err != nil {
		return err
	}
	if err := validateService("stack.services.search", cfg.Stack.Services.Search, validSearchServices); err != nil {
		return err
	}
	if err := validateService("stack.services.cache", cfg.Stack.Services.Cache, validCacheServices); err != nil {
		return err
	}
	if err := validateService("stack.services.queue", cfg.Stack.Services.Queue, validQueueServices); err != nil {
		return err
	}

	for name, remote := range cfg.Remotes {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("remote name cannot be empty")
		}
		if !IsValidRemoteName(name) {
			return fmt.Errorf("remote name '%s' is not a valid identifier (use lowercase letters, digits, hyphens, underscores)", name)
		}
		if strings.TrimSpace(remote.Host) == "" {
			return fmt.Errorf("remote '%s' is missing host", name)
		}
		if strings.TrimSpace(remote.User) == "" {
			return fmt.Errorf("remote '%s' is missing user", name)
		}
		if strings.TrimSpace(remote.Path) == "" {
			return fmt.Errorf("remote '%s' is missing path", name)
		}
		if remote.Port < 1 || remote.Port > 65535 {
			return fmt.Errorf("remote '%s' has invalid port %d", name, remote.Port)
		}
		if !IsSupportedRemoteAuthMethod(remote.Auth.Method) {
			return fmt.Errorf("remote '%s' has unsupported auth method '%s' (allowed: ssh-agent, keychain, keyfile)", name, remote.Auth.Method)
		}
	}

	for event, steps := range cfg.Hooks {
		if _, ok := allowedHookEvents[event]; !ok {
			return fmt.Errorf("unsupported hook event: %s", event)
		}
		for idx, step := range steps {
			if strings.TrimSpace(step.Run) == "" {
				return fmt.Errorf("hook %s has empty run command at index %d", event, idx)
			}
		}
	}

	return nil
}

func validateAuditConfig(config AuditConfig) error {
	if config.Lint.externalProviderKeyCollision != "" {
		return fmt.Errorf("audit.lint.external_providers keys collide after normalization as %q", config.Lint.externalProviderKeyCollision)
	}
	providerID := config.Lint.Provider
	if providerID == "" {
		providerID = "govard"
	}
	if !providerNamePattern.MatchString(providerID) {
		return fmt.Errorf("audit.lint.provider %q is invalid (allowed: lowercase letters, numbers, hyphen, underscore)", providerID)
	}
	for id, provider := range config.Lint.ExternalProviders {
		if !providerNamePattern.MatchString(id) {
			return fmt.Errorf("audit.lint.external_providers key %q is invalid (allowed: lowercase letters, numbers, hyphen, underscore)", id)
		}
		if provider.Type != "docker" {
			return fmt.Errorf("audit.lint.external_providers.%s has unsupported type %q (allowed: docker)", id, provider.Type)
		}
		if strings.TrimSpace(provider.Image) == "" {
			return fmt.Errorf("audit.lint.external_providers.%s is missing Docker image", id)
		}
		if len(provider.Command) == 0 {
			return fmt.Errorf("audit.lint.external_providers.%s has an empty command", id)
		}
		for index, argument := range provider.Command {
			if strings.TrimSpace(argument) == "" {
				return fmt.Errorf("audit.lint.external_providers.%s command argument %d is empty", id, index)
			}
		}
	}
	if providerID != "govard" {
		if _, ok := config.Lint.ExternalProviders[providerID]; !ok {
			return fmt.Errorf("audit.lint.provider %q is not a configured external provider", providerID)
		}
	}
	return nil
}

func validateService(field, value string, allowed map[string]struct{}) error {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if _, ok := allowed[value]; !ok {
		return fmt.Errorf("unsupported value for %s: %s", field, value)
	}
	return nil
}
