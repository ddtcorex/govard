package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
)

type OnboardInput struct {
	ProjectPath           string `json:"projectPath"`
	Framework             string `json:"framework"`
	FrameworkVersion      string `json:"frameworkVersion"`
	Domain                string `json:"domain"`
	CloneFromGit          bool   `json:"cloneFromGit"`
	GitProtocol           string `json:"gitProtocol"`
	GitURL                string `json:"gitURL"`
	ConfirmFolderOverride bool   `json:"confirmFolderOverride"`
	VarnishEnabled        bool   `json:"varnishEnabled"`
	RedisEnabled          bool   `json:"redisEnabled"`
	RabbitMQEnabled       bool   `json:"rabbitMQEnabled"`
	ElasticsearchEnabled  bool   `json:"elasticsearchEnabled"`
	ApplyOverrides        bool   `json:"applyOverrides"`
	SkipIDE               bool   `json:"skipIDE"`
}

type onboardingContextKey struct{}

var suppressOnboardingProgressKey onboardingContextKey

func pickProjectDirectoryInternal(p Platform) (string, error) {
	defaultDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultDir = home
	}

	path, err := p.ChooseDirectory("Select Project Directory", defaultDir)
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
		FrameworkVersion:     "",
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
	internalCtx := context.WithValue(context.Background(), suppressOnboardingProgressKey, true)
	return onboardProjectWithOptionsInternalWithContext(internalCtx, newDefaultPlatform(), input)
}

func onboardProjectWithOptionsInternalWithContext(
	ctx context.Context,
	p Platform,
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

	reportProgress := func(step string, progressMessage string) {
		if ctx == nil {
			return
		}
		if suppressed, _ := ctx.Value(suppressOnboardingProgressKey).(bool); suppressed {
			return
		}
		p.Emit(EventOnboardingProgress, OnboardingProgressPayload{
			Step:    strings.TrimSpace(step),
			Message: strings.TrimSpace(progressMessage),
		})
	}

	if input.CloneFromGit {
		if !input.ConfirmFolderOverride {
			err := fmt.Errorf("please confirm folder override before cloning from Git")
			category = "validation"
			message = err.Error()
			return "", err
		}
		if err := cloneProjectSourceFromGit(root, input.GitProtocol, input.GitURL, reportProgress); err != nil {
			category = "validation"
			message = err.Error()
			return "", err
		}
	}

	config, hasConfig, err := loadProjectConfigForOnboarding(root)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	ranInit := false
	if !hasConfig {
		migrateFrom := detectOnboardingMigrationSource(root)
		args := buildInitArgs(framework, input.FrameworkVersion, migrateFrom)
		reportProgress("govard.init", "Running Govard init...")
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
		engine.NormalizeConfig(&config, root)
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

	reportProgress("project.registry", "Registering project...")
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
	reportProgress("onboarding.complete", "Onboarding completed.")
	if ranInit {
		if message == "" {
			message = "project initialized and added"
		}
		return fmt.Sprintf("Project %s initialized and added.", entry.ProjectName), nil
	}
	if message == "" {
		message = "project added"
	}
	return fmt.Sprintf("Project %s added.", entry.ProjectName), nil
}

// OnboardingService methods

func (s *OnboardingService) PickProjectDirectory() (res string, err error) {
	defer RecoverPanic(&err, "PickProjectDirectory")
	return pickProjectDirectoryInternal(s.platform)
}

func (s *OnboardingService) OnboardProject(input OnboardInput) (res string, err error) {
	defer RecoverPanic(&err, "OnboardProject")
	return onboardProjectWithOptionsInternalWithContext(s.lifecycleContext(), s.platform, input)
}

func (s *OnboardingService) DetectMigrationSource(projectPath string) (res string, err error) {
	defer RecoverPanic(&err, "DetectMigrationSource")
	root, err := normalizeProjectPath(projectPath)
	if err != nil {
		return "", err
	}
	return detectOnboardingMigrationSource(root), nil
}

func loadProjectConfigForOnboarding(root string) (engine.Config, bool, error) {
	if !pathHasBaseConfig(root) {
		return engine.Config{}, false, nil
	}

	cfg, err := engine.LoadBaseConfigFromDir(root, true)
	if err != nil {
		return engine.Config{}, true, fmt.Errorf("load %s: %w", conventions.BaseConfigFile, err)
	}
	return cfg, true, nil
}

func buildInitArgs(framework string, frameworkVersion string, migrateFrom string) []string {
	args := []string{"init"}
	if normalized := normalizeOnboardingFramework(framework); normalized != "" {
		args = append(args, "--framework", normalized)
	}
	if normalized := strings.TrimSpace(frameworkVersion); normalized != "" {
		args = append(args, "--framework-version", normalized)
	}
	if normalized := normalizeOnboardingMigrationSource(migrateFrom); normalized != "" {
		args = append(args, "--migrate-from", normalized)
	}
	return args
}

func detectOnboardingMigrationSource(root string) string {
	if hasWardenProjectSignals(root) {
		return "warden"
	}
	return ""
}

func normalizeOnboardingMigrationSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "warden":
		return "warden"
	default:
		return ""
	}
}

func hasWardenProjectSignals(root string) bool {
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	if cleanRoot == "" {
		return false
	}

	if info, err := os.Stat(filepath.Join(cleanRoot, ".warden", "warden-env.yml")); err == nil && !info.IsDir() {
		return true
	}

	env := engine.ParseDotEnv(filepath.Join(cleanRoot, ".env"))
	for key := range env {
		normalized := strings.ToUpper(strings.TrimSpace(key))
		if strings.HasPrefix(normalized, "WARDEN_") || strings.HasPrefix(normalized, "REMOTE_") {
			return true
		}
	}

	return false
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
		config.Stack.Features.Cache = (cacheTarget != "none")
		changed = true
	}

	queueTarget := "none"
	if rabbitMQEnabled {
		queueTarget = "rabbitmq"
	}
	if config.Stack.Services.Queue != queueTarget {
		config.Stack.Services.Queue = queueTarget
		config.Stack.Features.Queue = (queueTarget != "none")
		changed = true
	}

	searchTarget := "none"
	if elasticsearchEnabled {
		searchTarget = "elasticsearch"
	}
	if config.Stack.Services.Search != searchTarget {
		config.Stack.Services.Search = searchTarget
		config.Stack.Features.Search = (searchTarget != "none")
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
