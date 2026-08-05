package desktop

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"

	"github.com/docker/docker/api/types/container"
)

// ResetStateForTest clears process-level caches used by desktop package.
func ResetStateForTest() {
	prefsMu.Lock()
	cachedPrefs = nil
	prefsMu.Unlock()
	runDesktopSelfUpdate = defaultRunDesktopSelfUpdate
	restartDesktopBinary = defaultRestartDesktopBinary
	desktopExecutablePath = os.Executable
	desktopBinaryLookPath = exec.LookPath
	desktopGovardLookPath = exec.LookPath
	desktopPrivilegedCommandLookPath = exec.LookPath
	runGovardCommandForDesktop = defaultRunGovardCommandForDesktop
	runGovardCommandForDesktopWithTimeout = defaultRunGovardCommandForDesktopWithTimeout
	startGovardCommandForDesktop = defaultStartGovardCommandForDesktop
	validateGitConnectionForDesktop = defaultValidateGitConnectionForDesktop
	cloneGitRepoForDesktop = defaultCloneGitRepoForDesktop
	openExternalURLForDesktop = defaultOpenExternalURLForDesktop
	runEnvironmentComposeForDesktop = defaultRunEnvironmentComposeForDesktop
	runGlobalServicesComposeForDesktop = defaultRunGlobalServicesComposeForDesktop
	ensureGlobalServicesForDesktop = defaultEnsureGlobalServicesForDesktop
	waitForGlobalProxyReadyForDesktop = defaultWaitForGlobalProxyReadyForDesktop
	refreshGlobalServiceRoutesForDesktop = defaultRefreshGlobalServiceRoutesForDesktop
	runHostPortProbeForDesktop = defaultRunHostPortProbeForDesktop
	chooseSaveFileForDesktop = defaultChooseSaveFileForDesktop
	writeLogFileForDesktop = defaultWriteLogFileForDesktop
}

// ResolveRequestedLogTargetsForTest exposes log target normalization for tests.
func ResolveRequestedLogTargetsForTest(service string, discovered []string) []string {
	return resolveRequestedLogTargets(service, discovered)
}

// PrefixServiceLogLinesForTest exposes service-prefix formatting for tests.
func PrefixServiceLogLinesForTest(service string, raw string) string {
	return prefixServiceLogLines(service, raw)
}

// SanitizeStreamLineForTest exposes streaming line sanitization for tests.
func SanitizeStreamLineForTest(raw []byte) string {
	return sanitizeStreamLine(raw)
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

// NormalizeShellForTest exposes shell normalization logic for tests.
func NormalizeShellForTest(shell string) string {
	return normalizeShell(shell)
}

// ParseDockerPublishedPortForTest exposes docker port output parsing for tests.
func ParseDockerPublishedPortForTest(raw string) (string, string, bool) {
	return parseDockerPublishedPort(raw)
}

// BuildDesktopDBClientURLForTest exposes local DB client URL formatting for tests.
func BuildDesktopDBClientURLForTest(
	scheme string,
	user string,
	pass string,
	host string,
	port string,
	db string,
) string {
	return buildDesktopDBClientURL(scheme, user, pass, host, port, db)
}

// BuildPMAOpenURLForTest exposes PMA deep-link formatting for tests.
func BuildPMAOpenURLForTest(project string, database string) string {
	return buildPMAOpenURL(project, database)
}

// ParseContainerIPAddressesForTest exposes container IP list parsing for tests.
func ParseContainerIPAddressesForTest(raw string) []string {
	return parseContainerIPAddresses(raw)
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
			URL:       strings.TrimSpace(snapshot.URL),
			Protected: engine.BoolPtr(snapshot.Protected),
			Capabilities: &engine.RemoteCapabilities{
				Files: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityFiles)),
				Media: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityMedia)),
				DB:    desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityDB)),
			},
			Auth: engine.RemoteAuth{
				Method: strings.TrimSpace(snapshot.AuthMethod),
			},
		}
	}
	return buildRemoteEntries(engineRemotes, map[string]string{})
}

// BuildRemoteEntriesWithLastSyncForTest exposes remote entry rendering with Last Sync labels.
func BuildRemoteEntriesWithLastSyncForTest(
	remotes map[string]RemoteConfigSnapshot,
	lastSyncByRemote map[string]string,
) []RemoteEntry {
	engineRemotes := map[string]engine.RemoteConfig{}
	for name, snapshot := range remotes {
		engineRemotes[name] = engine.RemoteConfig{
			Host:      strings.TrimSpace(snapshot.Host),
			User:      strings.TrimSpace(snapshot.User),
			Path:      strings.TrimSpace(snapshot.Path),
			Port:      snapshot.Port,
			URL:       strings.TrimSpace(snapshot.URL),
			Protected: engine.BoolPtr(snapshot.Protected),
			Capabilities: &engine.RemoteCapabilities{
				Files: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityFiles)),
				Media: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityMedia)),
				DB:    desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityDB)),
			},
			Auth: engine.RemoteAuth{
				Method: strings.TrimSpace(snapshot.AuthMethod),
			},
		}
	}
	return buildRemoteEntries(engineRemotes, lastSyncByRemote)
}

// BuildRemoteLastSyncLabelsFromEventsForTest exposes operation-event to last-sync label mapping.
func BuildRemoteLastSyncLabelsFromEventsForTest(
	project string,
	events []engine.OperationEvent,
	now time.Time,
) map[string]string {
	return buildRemoteLastSyncLabelsFromEvents(project, events, now)
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
	noNoise bool,
	compress bool,
) ([]string, error) {
	return buildRemoteSyncPlanArgsWithOptions(
		remoteName,
		preset,
		map[string]bool{
			"noNoise":  noNoise,
			"compress": compress,
		},
	)
}

// BuildPresetSyncOptionDefsForTest exposes preset option definitions for tests.
func BuildPresetSyncOptionDefsForTest(project, preset string) presetSyncOptions {
	return buildPresetSyncOptionDefs(project, preset)
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

// BuildServiceTargetsFromServicesForTest exposes service target selection for configured environments.
func BuildServiceTargetsFromServicesForTest(services []Service, serviceState map[string]string) []string {
	info := &projectInfo{
		serviceState: map[string]string{},
	}
	for target, state := range serviceState {
		info.serviceState[target] = state
	}
	return collectServiceTargetsFromServices(info, services)
}

// BuildRemoteAdminURLForTest exposes remote admin URL formatting for tests.
func BuildRemoteAdminURLForTest(remote RemoteConfigSnapshot, adminPath string) string {
	return buildRemoteAdminURLForDesktop(engine.RemoteConfig{
		Host: strings.TrimSpace(remote.Host),
		URL:  strings.TrimSpace(remote.URL),
	}, adminPath)
}

// ResolveRemoteNameForOpenForTest exposes remote lookup by name/environment alias for tests.
func ResolveRemoteNameForOpenForTest(
	remotes map[string]RemoteConfigSnapshot,
	requestedRemoteName string,
) (string, error) {
	engineRemotes := map[string]engine.RemoteConfig{}
	for name, snapshot := range remotes {
		engineRemotes[name] = engine.RemoteConfig{
			Host: strings.TrimSpace(snapshot.Host),
			User: strings.TrimSpace(snapshot.User),
			Path: strings.TrimSpace(snapshot.Path),
			Port: snapshot.Port,
			URL:  strings.TrimSpace(snapshot.URL),
			Capabilities: &engine.RemoteCapabilities{
				Files: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityFiles)),
				Media: desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityMedia)),
				DB:    desktopBoolPtr(containsCapability(snapshot.Capabilities, engine.RemoteCapabilityDB)),
			},
		}
	}

	resolved, _, err := resolveRemoteConfigForOpen(
		engine.Config{
			ProjectName: "test",
			Domain:      "example.test",
			Framework:   "magento2",
			Remotes:     engineRemotes,
		},
		requestedRemoteName,
	)
	if err != nil {
		return "", err
	}

	return resolved, nil
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

// SetRunGovardCommandForDesktopWithTimeoutForTest overrides the desktop timeout-aware govard command runner.
func SetRunGovardCommandForDesktopWithTimeoutForTest(fn func(root string, args []string, timeout time.Duration) (string, error)) func() {
	previous := runGovardCommandForDesktopWithTimeout
	if fn == nil {
		runGovardCommandForDesktopWithTimeout = defaultRunGovardCommandForDesktopWithTimeout
	} else {
		runGovardCommandForDesktopWithTimeout = fn
	}
	return func() {
		runGovardCommandForDesktopWithTimeout = previous
	}
}

// SetStartGovardCommandForDesktopForTest overrides the desktop background govard starter.
func SetStartGovardCommandForDesktopForTest(fn func(root string, args []string) error) func() {
	previous := startGovardCommandForDesktop
	if fn == nil {
		startGovardCommandForDesktop = defaultStartGovardCommandForDesktop
	} else {
		startGovardCommandForDesktop = fn
	}
	return func() {
		startGovardCommandForDesktop = previous
	}
}

// SetRunDesktopSelfUpdateForTest overrides desktop self-update execution.
func SetRunDesktopSelfUpdateForTest(fn func() (string, error)) func() {
	previous := runDesktopSelfUpdate
	if fn == nil {
		runDesktopSelfUpdate = defaultRunDesktopSelfUpdate
	} else {
		runDesktopSelfUpdate = fn
	}
	return func() {
		runDesktopSelfUpdate = previous
	}
}

// SanitizeDesktopSelfUpdateOutputForTest exposes self-update output sanitization.
func SanitizeDesktopSelfUpdateOutputForTest(raw string) string {
	return sanitizeDesktopSelfUpdateOutput(raw)
}

// SummarizeDesktopSelfUpdateErrorForTest exposes self-update error summarization.
func SummarizeDesktopSelfUpdateErrorForTest(runErr error, sanitizedOutput string) string {
	return summarizeDesktopSelfUpdateError(runErr, sanitizedOutput)
}

// BuildDesktopElevatedSelfUpdateArgsForTest exposes elevated pkexec argument construction for desktop update.
func BuildDesktopElevatedSelfUpdateArgsForTest(envBinary, govardBinary, desktopTarget string) []string {
	return buildDesktopElevatedSelfUpdateArgs(envBinary, govardBinary, desktopTarget)
}

// ResolveGovardBinaryForDesktopUpdateForTest exposes govard binary resolution for desktop update.
func ResolveGovardBinaryForDesktopUpdateForTest() (string, error) {
	return resolveGovardBinaryForDesktopUpdate()
}

// ResolveDesktopBinaryForSelfUpdateTargetForTest exposes desktop target resolution for self-update.
func ResolveDesktopBinaryForSelfUpdateTargetForTest() string {
	return resolveDesktopBinaryForSelfUpdateTarget()
}

// SetDesktopGovardLookPathForUpdateForTest overrides govard PATH lookup for desktop update.
func SetDesktopGovardLookPathForUpdateForTest(fn func(file string) (string, error)) func() {
	previous := desktopGovardLookPath
	if fn == nil {
		desktopGovardLookPath = exec.LookPath
	} else {
		desktopGovardLookPath = fn
	}
	return func() {
		desktopGovardLookPath = previous
	}
}

// SetDesktopPrivilegedCommandLookPathForUpdateForTest overrides privileged command lookup for desktop update.
func SetDesktopPrivilegedCommandLookPathForUpdateForTest(fn func(file string) (string, error)) func() {
	previous := desktopPrivilegedCommandLookPath
	if fn == nil {
		desktopPrivilegedCommandLookPath = exec.LookPath
	} else {
		desktopPrivilegedCommandLookPath = fn
	}
	return func() {
		desktopPrivilegedCommandLookPath = previous
	}
}

// SetRestartDesktopBinaryForTest overrides desktop relaunch command execution.
func SetRestartDesktopBinaryForTest(fn func(binaryPath string) error) func() {
	previous := restartDesktopBinary
	if fn == nil {
		restartDesktopBinary = defaultRestartDesktopBinary
	} else {
		restartDesktopBinary = fn
	}
	return func() {
		restartDesktopBinary = previous
	}
}

// SetDesktopExecutablePathForRestartForTest overrides os.Executable usage in desktop restart flow.
func SetDesktopExecutablePathForRestartForTest(fn func() (string, error)) func() {
	previous := desktopExecutablePath
	if fn == nil {
		desktopExecutablePath = os.Executable
	} else {
		desktopExecutablePath = fn
	}
	return func() {
		desktopExecutablePath = previous
	}
}

// SetDesktopBinaryLookPathForRestartForTest overrides desktop binary lookup in restart flow.
func SetDesktopBinaryLookPathForRestartForTest(fn func(file string) (string, error)) func() {
	previous := desktopBinaryLookPath
	if fn == nil {
		desktopBinaryLookPath = exec.LookPath
	} else {
		desktopBinaryLookPath = fn
	}
	return func() {
		desktopBinaryLookPath = previous
	}
}

// SetValidateGitConnectionForDesktopForTest overrides git connection validation in onboarding flow.
func SetValidateGitConnectionForDesktopForTest(fn func(protocol string, repoURL string) error) func() {
	previous := validateGitConnectionForDesktop
	if fn == nil {
		validateGitConnectionForDesktop = defaultValidateGitConnectionForDesktop
	} else {
		validateGitConnectionForDesktop = fn
	}
	return func() {
		validateGitConnectionForDesktop = previous
	}
}

// SetCloneGitRepoForDesktopForTest overrides git clone behavior in onboarding flow.
func SetCloneGitRepoForDesktopForTest(fn func(repoURL string, destination string) error) func() {
	previous := cloneGitRepoForDesktop
	if fn == nil {
		cloneGitRepoForDesktop = defaultCloneGitRepoForDesktop
	} else {
		cloneGitRepoForDesktop = fn
	}
	return func() {
		cloneGitRepoForDesktop = previous
	}
}

// SetRunGlobalServicesComposeForDesktopForTest overrides the global compose runner.
func SetRunGlobalServicesComposeForDesktopForTest(fn func(args ...string) (string, error)) func() {
	previous := runGlobalServicesComposeForDesktop
	if fn == nil {
		runGlobalServicesComposeForDesktop = defaultRunGlobalServicesComposeForDesktop
	} else {
		runGlobalServicesComposeForDesktop = fn
	}
	return func() {
		runGlobalServicesComposeForDesktop = previous
	}
}

// SetRunEnvironmentComposeForDesktopForTest overrides project compose execution for desktop environment actions.
func SetRunEnvironmentComposeForDesktopForTest(fn func(dir string, args []string) error) func() {
	previous := runEnvironmentComposeForDesktop
	if fn == nil {
		runEnvironmentComposeForDesktop = defaultRunEnvironmentComposeForDesktop
	} else {
		runEnvironmentComposeForDesktop = fn
	}
	return func() {
		runEnvironmentComposeForDesktop = previous
	}
}

// SetEnsureGlobalServicesForDesktopForTest overrides global service compose readiness checks.
func SetEnsureGlobalServicesForDesktopForTest(fn func() error) func() {
	previous := ensureGlobalServicesForDesktop
	if fn == nil {
		ensureGlobalServicesForDesktop = defaultEnsureGlobalServicesForDesktop
	} else {
		ensureGlobalServicesForDesktop = fn
	}
	return func() {
		ensureGlobalServicesForDesktop = previous
	}
}

// SetWaitForGlobalProxyReadyForDesktopForTest overrides global proxy readiness checks.
func SetWaitForGlobalProxyReadyForDesktopForTest(
	fn func(ctx context.Context, timeout time.Duration) bool,
) func() {
	previous := waitForGlobalProxyReadyForDesktop
	if fn == nil {
		waitForGlobalProxyReadyForDesktop = defaultWaitForGlobalProxyReadyForDesktop
	} else {
		waitForGlobalProxyReadyForDesktop = fn
	}
	return func() {
		waitForGlobalProxyReadyForDesktop = previous
	}
}

// SetRefreshGlobalServiceRoutesForDesktopForTest overrides route refresh after global start/restart.
func SetRefreshGlobalServiceRoutesForDesktopForTest(fn func() error) func() {
	previous := refreshGlobalServiceRoutesForDesktop
	if fn == nil {
		refreshGlobalServiceRoutesForDesktop = defaultRefreshGlobalServiceRoutesForDesktop
	} else {
		refreshGlobalServiceRoutesForDesktop = fn
	}
	return func() {
		refreshGlobalServiceRoutesForDesktop = previous
	}
}

// SetRunHostPortProbeForDesktopForTest overrides host port probe commands (lsof/ss).
func SetRunHostPortProbeForDesktopForTest(
	fn func(binary string, args ...string) (string, error),
) func() {
	previous := runHostPortProbeForDesktop
	if fn == nil {
		runHostPortProbeForDesktop = defaultRunHostPortProbeForDesktop
	} else {
		runHostPortProbeForDesktop = fn
	}
	return func() {
		runHostPortProbeForDesktop = previous
	}
}

// SetChooseSaveFileForDesktopForTest overrides the desktop save file picker for tests.
func SetChooseSaveFileForDesktopForTest(
	fn func(ctx context.Context, title string, defaultDir string, defaultFilename string) (string, error),
) func() {
	previous := chooseSaveFileForDesktop
	if fn == nil {
		chooseSaveFileForDesktop = defaultChooseSaveFileForDesktop
	} else {
		chooseSaveFileForDesktop = fn
	}
	return func() {
		chooseSaveFileForDesktop = previous
	}
}

// SetWriteLogFileForDesktopForTest overrides log file write behavior in export flow.
func SetWriteLogFileForDesktopForTest(
	fn func(path string, data []byte, perm os.FileMode) error,
) func() {
	previous := writeLogFileForDesktop
	if fn == nil {
		writeLogFileForDesktop = defaultWriteLogFileForDesktop
	} else {
		writeLogFileForDesktop = fn
	}
	return func() {
		writeLogFileForDesktop = previous
	}
}

// ResolveGlobalServiceForTest exposes global service definitions for tests.
func ResolveGlobalServiceForTest(serviceID string) (GlobalService, bool) {
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return GlobalService{}, false
	}
	return GlobalService{
		ID:             spec.ID,
		Name:           spec.Name,
		ComposeService: spec.ComposeService,
		ContainerName:  spec.ContainerName,
		Openable:       spec.URLHost != "",
	}, true
}

// DeriveGlobalContainerStatusForTest exposes container state normalization for tests.
func DeriveGlobalContainerStatusForTest(state string, statusText string) (string, string, bool) {
	return deriveGlobalContainerStatus(state, statusText)
}

// DetectRoutingPublishedPortBindingWarningsForTest exposes running-state port publish checks for routing services.
func DetectRoutingPublishedPortBindingWarningsForTest(
	services []GlobalService,
	containersByName map[string]container.Summary,
) []string {
	return detectRoutingPublishedPortBindingWarnings(services, containersByName)
}

// DetectDockerPortConflictWarningsForTest exposes docker listener conflict detection for tests.
func DetectDockerPortConflictWarningsForTest(containers []container.Summary) []string {
	return detectDockerPortConflictWarnings(containers)
}

// BuildHostPortConflictWarningsFromLsofForTest exposes lsof output parsing for host listeners.
func BuildHostPortConflictWarningsFromLsofForTest(output string, port int, protocol string) []string {
	owners := parseLsofPortOwners(output, port, protocol)
	return formatHostPortConflictWarnings(owners)
}

// BuildHostPortConflictWarningsFromSSForTest exposes ss output parsing for host listeners.
func BuildHostPortConflictWarningsFromSSForTest(output string, port int, protocol string) []string {
	owners := parseSSPortOwners(output, port, protocol)
	return formatHostPortConflictWarnings(owners)
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
	frameworkVersion ...string,
) (string, error) {
	version := ""
	if len(frameworkVersion) > 0 {
		version = strings.TrimSpace(frameworkVersion[0])
	}
	return onboardProjectWithOptionsInternal(OnboardInput{
		ProjectPath:          projectPath,
		Framework:            framework,
		FrameworkVersion:     version,
		Domain:               domain,
		VarnishEnabled:       varnishEnabled,
		RedisEnabled:         redisEnabled,
		RabbitMQEnabled:      rabbitMQEnabled,
		ElasticsearchEnabled: elasticsearchEnabled,
		ApplyOverrides:       true,
		SkipIDE:              false,
	})
}

// OnboardProjectFromGitForPathForTest exposes onboarding flow with clone-from-git enabled.
func OnboardProjectFromGitForPathForTest(
	projectPath string,
	framework string,
	gitProtocol string,
	gitURL string,
) (string, error) {
	return OnboardProjectFromGitWithConfirmationForPathForTest(
		projectPath,
		framework,
		gitProtocol,
		gitURL,
		true,
	)
}

// OnboardProjectFromGitWithConfirmationForPathForTest exposes git onboarding flow with explicit override confirmation.
func OnboardProjectFromGitWithConfirmationForPathForTest(
	projectPath string,
	framework string,
	gitProtocol string,
	gitURL string,
	confirmFolderOverride bool,
) (string, error) {
	return onboardProjectWithOptionsInternal(OnboardInput{
		ProjectPath:           projectPath,
		Framework:             framework,
		FrameworkVersion:      "",
		Domain:                "",
		CloneFromGit:          true,
		GitProtocol:           gitProtocol,
		GitURL:                gitURL,
		ConfirmFolderOverride: confirmFolderOverride,
		VarnishEnabled:        false,
		RedisEnabled:          false,
		RabbitMQEnabled:       false,
		ElasticsearchEnabled:  false,
		ApplyOverrides:        false,
		SkipIDE:               false,
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

func desktopBoolPtr(v bool) *bool {
	if v {
		return nil
	}
	return engine.BoolPtr(false)
}
