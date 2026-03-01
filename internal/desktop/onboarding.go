package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
)

const desktopBootstrapSyncEnvVar = "GOVARD_DESKTOP_BOOTSTRAP_SYNC"

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
		migrateFrom := detectOnboardingMigrationSource(root)
		args := buildInitArgs(framework, migrateFrom)
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

	bootstrapSummary := ""
	if stagingRemote, ok := resolveStagingBootstrapRemote(config.Remotes); ok {
		if shouldRunDesktopBootstrapSynchronously() {
			bootstrapOutput, bootstrapErr := runGovardCommandForDesktop(root, []string{
				"bootstrap",
				"--environment",
				stagingRemote,
			})
			if bootstrapErr != nil {
				bootstrapSummary = fmt.Sprintf(
					" Auto bootstrap from '%s' failed (%v). Run `govard bootstrap --environment %s` manually.",
					stagingRemote,
					bootstrapErr,
					stagingRemote,
				)
				message = "project added; staging bootstrap failed"
			} else if strings.TrimSpace(bootstrapOutput) != "" {
				bootstrapSummary = fmt.Sprintf(" Auto bootstrap from '%s' completed: %s", stagingRemote, strings.TrimSpace(bootstrapOutput))
			} else {
				bootstrapSummary = fmt.Sprintf(" Auto bootstrap from '%s' completed.", stagingRemote)
			}
		} else {
			runStagingBootstrapInBackground(root, entry.ProjectName, stagingRemote)
			bootstrapSummary = fmt.Sprintf(" Auto bootstrap from '%s' started in background.", stagingRemote)
		}
	}

	status = engine.OperationStatusSuccess
	category = ""
	if ranInit {
		if message == "" {
			message = "project initialized and added"
		}
		return fmt.Sprintf("Project %s initialized and added.%s", entry.ProjectName, bootstrapSummary), nil
	}
	if message == "" {
		message = "project added"
	}
	return fmt.Sprintf("Project %s added.%s", entry.ProjectName, bootstrapSummary), nil
}

func shouldRunDesktopBootstrapSynchronously() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(desktopBootstrapSyncEnvVar)))
	return value == "1" || value == "true" || value == "yes"
}

func runStagingBootstrapInBackground(root string, project string, stagingRemote string) {
	go func() {
		startedAt := time.Now()
		status := engine.OperationStatusFailure
		category := "runtime"
		message := ""
		defer func() {
			writeDesktopOperationEvent(
				"desktop.project.bootstrap",
				status,
				project,
				stagingRemote,
				"",
				message,
				category,
				time.Since(startedAt),
			)
		}()

		output, err := runGovardCommandForDesktop(root, []string{
			"bootstrap",
			"--environment",
			stagingRemote,
		})
		if err != nil {
			message = fmt.Sprintf("auto bootstrap from %s failed: %v", stagingRemote, err)
			return
		}
		status = engine.OperationStatusSuccess
		category = ""
		trimmedOutput := strings.TrimSpace(output)
		if trimmedOutput == "" {
			message = fmt.Sprintf("auto bootstrap from %s completed", stagingRemote)
			return
		}
		message = fmt.Sprintf("auto bootstrap from %s completed: %s", stagingRemote, trimmedOutput)
	}()
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

func buildInitArgs(framework string, migrateFrom string) []string {
	args := []string{"init"}
	if normalized := normalizeOnboardingFramework(framework); normalized != "" {
		args = append(args, "--framework", normalized)
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

func resolveStagingBootstrapRemote(remotes map[string]engine.RemoteConfig) (string, bool) {
	if len(remotes) == 0 {
		return "", false
	}

	if _, ok := remotes[engine.RemoteEnvStaging]; ok {
		return engine.RemoteEnvStaging, true
	}

	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if engine.NormalizeRemoteEnvironment(name) == engine.RemoteEnvStaging {
			return name, true
		}
	}

	return "", false
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
