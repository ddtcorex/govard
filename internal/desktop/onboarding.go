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

type OnboardInput struct {
	ProjectPath          string `json:"projectPath"`
	Framework            string `json:"framework"`
	Domain               string `json:"domain"`
	VarnishEnabled       bool   `json:"varnishEnabled"`
	RedisEnabled         bool   `json:"redisEnabled"`
	RabbitMQEnabled      bool   `json:"rabbitMQEnabled"`
	ElasticsearchEnabled bool   `json:"elasticsearchEnabled"`
	ApplyOverrides       bool   `json:"applyOverrides"`
	SkipIDE              bool   `json:"skipIDE"`
}

func pickProjectDirectoryInternal(ctx context.Context) (string, error) {
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

func onboardProject(projectPath string, framework string) (string, error) {
	return onboardProjectWithOptionsInternal(OnboardInput{
		ProjectPath:          projectPath,
		Framework:            framework,
		Domain:               "",
		VarnishEnabled:       false,
		RedisEnabled:         false,
		RabbitMQEnabled:      false,
		ElasticsearchEnabled: false,
		ApplyOverrides:       false,
		SkipIDE:              false,
	})
}

func onboardProjectWithOptionsInternal(
	input OnboardInput,
) (string, error) {
	projectPath := input.ProjectPath
	framework := input.Framework
	varnishEnabled := input.VarnishEnabled
	redisEnabled := input.RedisEnabled
	rabbitMQEnabled := input.RabbitMQEnabled
	elasticsearchEnabled := input.ElasticsearchEnabled
	applyOverrides := input.ApplyOverrides

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
		args := buildInitArgs(framework)
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
			message = ".govard.yml not found after init"
			return "", fmt.Errorf(".govard.yml not found after init")
		}
	}

	if applyOverrides &&
		applyOnboardingOverrides(
			&config,
			input.Domain,
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
	if err := validateUniqueOnboardingDomain(root, entry.Domain); err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}
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

// OnboardingService methods

func (s *OnboardingService) PickProjectDirectory() (string, error) {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return pickProjectDirectoryInternal(ctx)
}

func (s *OnboardingService) OnboardProject(input OnboardInput) (string, error) {
	return onboardProjectWithOptionsInternal(input)
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

func buildInitArgs(framework string) []string {
	args := []string{"init"}
	if normalized := normalizeOnboardingFramework(framework); normalized != "" {
		args = append(args, "--framework", normalized)
	}
	return args
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
		if config.Domain != normalizedDomain {
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
	if config.Stack.Services.Cache != cacheTarget {
		config.Stack.Services.Cache = cacheTarget
		changed = true
	}

	queueTarget := "none"
	if rabbitMQEnabled {
		queueTarget = "rabbitmq"
	}
	if config.Stack.Services.Queue != queueTarget {
		config.Stack.Services.Queue = queueTarget
		changed = true
	}

	searchTarget := "none"
	if elasticsearchEnabled {
		searchTarget = "elasticsearch"
	}
	if config.Stack.Services.Search != searchTarget {
		config.Stack.Services.Search = searchTarget
		changed = true
	}

	return changed
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
		Framework:   strings.TrimSpace(strings.ToLower(config.Framework)),
		LastSeenAt:  time.Now().UTC(),
		LastCommand: strings.TrimSpace(strings.ToLower(command)),
	}
}

func validateUniqueOnboardingDomain(projectPath string, domain string) error {
	normalizedDomain := strings.TrimSpace(strings.ToLower(domain))
	if normalizedDomain == "" {
		return nil
	}

	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return fmt.Errorf("read project registry: %w", err)
	}

	cleanPath := filepath.Clean(strings.TrimSpace(projectPath))
	for _, entry := range entries {
		if filepath.Clean(strings.TrimSpace(entry.Path)) == cleanPath {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(entry.Domain), normalizedDomain) {
			return fmt.Errorf(
				"domain %s is already used by project %s",
				normalizedDomain,
				strings.TrimSpace(entry.ProjectName),
			)
		}

		for _, extraDomain := range entry.ExtraDomains {
			if strings.EqualFold(strings.TrimSpace(extraDomain), normalizedDomain) {
				return fmt.Errorf(
					"domain %s is already used by project %s",
					normalizedDomain,
					strings.TrimSpace(entry.ProjectName),
				)
			}
		}
	}

	return nil
}
