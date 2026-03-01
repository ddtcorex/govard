package desktop

import (
	"strings"

	"govard/internal/engine"
)

// ResetStateForTest clears process-level caches used by desktop package.
func ResetStateForTest() {
	prefsMu.Lock()
	cachedPrefs = nil
	prefsMu.Unlock()
	runGovardCommandForDesktop = defaultRunGovardCommandForDesktop
}

// ResolveRequestedLogTargetsForTest exposes log target normalization for tests.
func ResolveRequestedLogTargetsForTest(service string, discovered []string) []string {
	return resolveRequestedLogTargets(service, discovered)
}

// PrefixServiceLogLinesForTest exposes service-prefix formatting for tests.
func PrefixServiceLogLinesForTest(service string, raw string) string {
	return prefixServiceLogLines(service, raw)
}

// ResolveShellServiceNameForTest exposes shell target service resolution for tests.
func ResolveShellServiceNameForTest(requested string, available []string) string {
	info := &projectInfo{services: map[string]bool{}}
	for _, service := range available {
		trimmed := strings.TrimSpace(service)
		if trimmed == "" {
			continue
		}
		info.services[trimmed] = true
	}
	return resolveShellServiceName(info, requested)
}

// NormalizeOnboardingDomainForTest exposes domain normalization for tests.
func NormalizeOnboardingDomainForTest(domain string) string {
	return normalizeOnboardingDomain(domain)
}

// BuildOperationNotificationForTest exposes operation notification formatting for tests.
func BuildOperationNotificationForTest(event engine.OperationEvent) (OperationNotification, bool) {
	return buildOperationNotification(event)
}

// SelectOperationEventsSinceForTest exposes operation event cursor logic for tests.
func SelectOperationEventsSinceForTest(events []engine.OperationEvent, cursor string) ([]engine.OperationEvent, string) {
	return selectOperationEventsSince(events, cursor)
}

// OperationEventSignatureForTest exposes operation event signature generation for tests.
func OperationEventSignatureForTest(event engine.OperationEvent) string {
	return operationEventSignature(event)
}

// BuildRemoteEntriesForTest exposes remote snapshot rendering for tests.
func BuildRemoteEntriesForTest(remotes map[string]RemoteConfigSnapshot) []RemoteEntry {
	engineRemotes := map[string]engine.RemoteConfig{}
	for name, snapshot := range remotes {
		engineRemotes[name] = engine.RemoteConfig{
			Host:      strings.TrimSpace(snapshot.Host),
			User:      strings.TrimSpace(snapshot.User),
			Path:      strings.TrimSpace(snapshot.Path),
			Port:      snapshot.Port,
			Protected: engine.BoolPtr(snapshot.Protected),
			Capabilities: engine.RemoteCapabilities{
				Files:  containsCapability(snapshot.Capabilities, engine.RemoteCapabilityFiles),
				Media:  containsCapability(snapshot.Capabilities, engine.RemoteCapabilityMedia),
				DB:     containsCapability(snapshot.Capabilities, engine.RemoteCapabilityDB),
				Deploy: containsCapability(snapshot.Capabilities, engine.RemoteCapabilityDeploy),
			},
			Auth: engine.RemoteAuth{
				Method: strings.TrimSpace(snapshot.AuthMethod),
			},
		}
	}
	return buildRemoteEntries(engineRemotes)
}

// NormalizeRemoteSyncPresetForTest exposes preset normalization for tests.
func NormalizeRemoteSyncPresetForTest(preset string) (string, error) {
	return normalizeRemoteSyncPreset(preset)
}

// BuildRemoteSyncPlanArgsForTest exposes sync preset command argument generation for tests.
func BuildRemoteSyncPlanArgsForTest(remoteName string, preset string) ([]string, error) {
	return buildRemoteSyncPlanArgs(remoteName, preset)
}

// BuildRemoteSyncPlanArgsWithOptionsForTest exposes sync preset argument generation with desktop sync toggles.
func BuildRemoteSyncPlanArgsWithOptionsForTest(
	remoteName string,
	preset string,
	sanitize bool,
	excludeLogs bool,
	compress bool,
) ([]string, error) {
	return buildRemoteSyncPlanArgsWithOptions(
		remoteName,
		preset,
		map[string]bool{
			"sanitize":    sanitize,
			"excludeLogs": excludeLogs,
			"compress":    compress,
		},
	)
}

// BuildPresetSyncOptionDefsForTest exposes preset option definitions for tests.
func BuildPresetSyncOptionDefsForTest(preset string) presetSyncOptions {
	return buildPresetSyncOptionDefs(preset)
}

// BuildBootstrapArgsWithOptionsForTest exposes bootstrap arguments builder for tests.
func BuildBootstrapArgsWithOptionsForTest(remoteName string, options map[string]bool, planOnly bool) ([]string, error) {
	return buildBootstrapArgsWithOptions(remoteName, options, planOnly)
}

// BuildDerivedServicesForTest exposes desktop service rendering from config/state.
func BuildDerivedServicesForTest(config engine.Config, serviceState map[string]string) []Service {
	return deriveServices(config, serviceState)
}

// BuildFallbackServicesForTest exposes fallback service rendering from discovered targets.
func BuildFallbackServicesForTest(services map[string]bool, serviceState map[string]string) []Service {
	return fallbackServices(services, serviceState)
}

// ListProjectRemotesForPathForTest exposes path-based remotes loading for tests.
func ListProjectRemotesForPathForTest(root string) (RemoteSnapshot, error) {
	return listProjectRemotesByPath(root)
}

// SetRunGovardCommandForDesktopForTest overrides the desktop govard command runner.
func SetRunGovardCommandForDesktopForTest(fn func(root string, args []string) (string, error)) func() {
	previous := runGovardCommandForDesktop
	if fn == nil {
		runGovardCommandForDesktop = defaultRunGovardCommandForDesktop
	} else {
		runGovardCommandForDesktop = fn
	}
	return func() {
		runGovardCommandForDesktop = previous
	}
}

// OnboardProjectForPathForTest exposes onboarding flow for tests.
func OnboardProjectForPathForTest(projectPath string, framework string) (string, error) {
	return onboardProject(projectPath, framework)
}

// OnboardProjectWithOptionsForPathForTest exposes onboarding flow with desktop overrides.
func OnboardProjectWithOptionsForPathForTest(
	projectPath string,
	framework string,
	domain string,
	varnishEnabled bool,
	redisEnabled bool,
	rabbitMQEnabled bool,
	elasticsearchEnabled bool,
) (string, error) {
	return onboardProjectWithOptionsInternal(OnboardInput{
		ProjectPath:          projectPath,
		Framework:            framework,
		Domain:               domain,
		VarnishEnabled:       varnishEnabled,
		RedisEnabled:         redisEnabled,
		RabbitMQEnabled:      rabbitMQEnabled,
		ElasticsearchEnabled: elasticsearchEnabled,
		ApplyOverrides:       true,
		SkipIDE:              false,
	})
}

// LooksLikeGovardForTest exposes desktop project filtering for tests.
func LooksLikeGovardForTest(
	project string,
	services []string,
	configFiles []string,
	configLoaded bool,
) bool {
	info := &projectInfo{
		name:         strings.TrimSpace(project),
		services:     map[string]bool{},
		configFiles:  append([]string{}, configFiles...),
		configLoaded: configLoaded,
	}
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		info.services[service] = true
	}
	return looksLikeGovard(info)
}

func containsCapability(capabilities []string, name string) bool {
	for _, capability := range capabilities {
		if strings.EqualFold(strings.TrimSpace(capability), name) {
			return true
		}
	}
	return false
}
