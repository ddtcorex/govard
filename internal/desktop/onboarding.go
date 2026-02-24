package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
)

func pickProjectDirectory(ctx context.Context) (string, error) {
	defaultDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultDir = home
	}

	path, err := chooseDirectory(ctx, "Select Project Directory", defaultDir)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	return filepath.Clean(path), nil
}

func onboardProject(projectPath string, recipe string) (string, error) {
	return onboardProjectWithOptions(
		projectPath,
		recipe,
		"",
		false,
		false,
		false,
		false,
		false,
	)
}

func onboardProjectWithOptions(
	projectPath string,
	recipe string,
	domain string,
	varnishEnabled bool,
	redisEnabled bool,
	rabbitMQEnabled bool,
	elasticsearchEnabled bool,
	applyOverrides bool,
) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	project := strings.TrimSpace(projectPath)
	defer func() {
		writeDesktopOperationEvent(
			"desktop.project.onboard",
			status,
			project,
			project,
			"",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	root, err := normalizeProjectPath(projectPath)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}
	project = root

	config, hasConfig, err := loadProjectConfigForOnboarding(root)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	ranInit := false
	if !hasConfig {
		args := buildInitArgs(recipe)
		if _, err := runGovardCommandForDesktop(root, args); err != nil {
			message = err.Error()
			return "", fmt.Errorf("project init failed: %w", err)
		}
		ranInit = true

		config, hasConfig, err = loadProjectConfigForOnboarding(root)
		if err != nil {
			message = err.Error()
			return "", err
		}
		if !hasConfig {
			message = "govard.yml not found after init"
			return "", fmt.Errorf("govard.yml not found after init")
		}
	}

	if applyOverrides &&
		applyOnboardingOverrides(
			&config,
			domain,
			varnishEnabled,
			redisEnabled,
			rabbitMQEnabled,
			elasticsearchEnabled,
		) {
		engine.NormalizeConfig(&config)
		if err := writeBaseConfig(root, config); err != nil {
			message = err.Error()
			return "", err
		}
		if err := engine.RenderBlueprint(root, config); err != nil {
			// Keep onboarding resilient: config and registry update are primary outcomes.
			// Compose render can be retried later via regular runtime commands.
			message = "compose render skipped: " + err.Error()
		}
	}

	entry := buildProjectRegistryEntry(root, config, "desktop-onboard")
	project = entry.ProjectName
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		message = err.Error()
		return "", err
	}

	status = engine.OperationStatusSuccess
	category = ""
	if ranInit {
		message = "project initialized and added"
		return fmt.Sprintf("Project %s initialized and added.", entry.ProjectName), nil
	}
	message = "project added"
	return fmt.Sprintf("Project %s added.", entry.ProjectName), nil
}

func normalizeProjectPath(projectPath string) (string, error) {
	path := strings.TrimSpace(projectPath)
	if path == "" {
		return "", fmt.Errorf("project path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", absPath)
	}

	return filepath.Clean(absPath), nil
}

func loadProjectConfigForOnboarding(root string) (engine.Config, bool, error) {
	if !pathHasBaseConfig(root) {
		return engine.Config{}, false, nil
	}

	cfg, err := engine.LoadBaseConfigFromDir(root, true)
	if err != nil {
		return engine.Config{}, true, fmt.Errorf("load %s: %w", engine.BaseConfigFile, err)
	}
	return cfg, true, nil
}

func buildInitArgs(recipe string) []string {
	args := []string{"init"}
	if normalized := normalizeOnboardingRecipe(recipe); normalized != "" {
		args = append(args, "--recipe", normalized)
	}
	return args
}

func normalizeOnboardingRecipe(recipe string) string {
	switch strings.ToLower(strings.TrimSpace(recipe)) {
	case "", "auto", "detect":
		return ""
	case "m2":
		return "magento2"
	case "m1":
		return "magento1"
	case "wp":
		return "wordpress"
	default:
		return strings.ToLower(strings.TrimSpace(recipe))
	}
}

func applyOnboardingOverrides(
	config *engine.Config,
	domain string,
	varnishEnabled bool,
	redisEnabled bool,
	rabbitMQEnabled bool,
	elasticsearchEnabled bool,
) bool {
	if config == nil {
		return false
	}

	changed := false

	if normalizedDomain := normalizeOnboardingDomain(domain); normalizedDomain != "" {
		if strings.TrimSpace(config.Domain) != normalizedDomain {
			config.Domain = normalizedDomain
			changed = true
		}
	}

	if config.Stack.Features.Varnish != varnishEnabled {
		config.Stack.Features.Varnish = varnishEnabled
		changed = true
	}

	cacheTarget := "none"
	if redisEnabled {
		cacheTarget = "redis"
	}
	if strings.ToLower(strings.TrimSpace(config.Stack.Services.Cache)) != cacheTarget {
		config.Stack.Services.Cache = cacheTarget
		changed = true
	}

	queueTarget := "none"
	if rabbitMQEnabled {
		queueTarget = "rabbitmq"
	}
	if strings.ToLower(strings.TrimSpace(config.Stack.Services.Queue)) != queueTarget {
		config.Stack.Services.Queue = queueTarget
		changed = true
	}

	searchTarget := "none"
	if elasticsearchEnabled {
		searchTarget = "elasticsearch"
	}
	if strings.ToLower(strings.TrimSpace(config.Stack.Services.Search)) != searchTarget {
		config.Stack.Services.Search = searchTarget
		changed = true
	}

	return changed
}

func normalizeOnboardingDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	if normalized == "" {
		return ""
	}
	if !strings.Contains(normalized, ".") {
		normalized += ".test"
	}
	return normalized
}

func buildProjectRegistryEntry(root string, config engine.Config, command string) engine.ProjectRegistryEntry {
	projectName := strings.TrimSpace(config.ProjectName)
	if projectName == "" {
		projectName = filepath.Base(root)
	}

	domain := strings.TrimSpace(config.Domain)
	if domain == "" {
		domain = projectName + ".test"
	}

	return engine.ProjectRegistryEntry{
		Path:        filepath.Clean(root),
		ProjectName: projectName,
		Domain:      domain,
		Recipe:      strings.TrimSpace(strings.ToLower(config.Recipe)),
		LastSeenAt:  time.Now().UTC(),
		LastCommand: strings.TrimSpace(strings.ToLower(command)),
	}
}
