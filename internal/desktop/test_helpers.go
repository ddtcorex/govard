package desktop

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"govard/internal/audit"
	"govard/internal/deploy"
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
	_ = SetDeployHostResolverForTest(nil)
	_ = SetDeployListReleasesForTest(nil)
	_ = SetDeployLiveReleaseForTest(nil)
	_ = SetDeployPublishStrategyForTest(nil)
	_ = SetDeployCoreCheckForTest(nil)
	_ = SetDeployTimeoutsForTest(defaultDeployPlanTimeout, defaultDeployReadTimeout)
	_ = SetDeployMutateTimeoutForTest(defaultDeployMutateTimeout)
	_ = SetDeploySelectRollbackTargetForTest(nil)
	_ = SetDeployRunStepForTest(nil)
	_ = SetDeployExecutorRunForTest(nil)
	_ = SetDeploySandboxRuntimeForTest(nil)
	_ = SetDeploySandboxProbeForTest(nil)
	_ = SetLaunchInTerminalForTest(nil)
	_ = SetDeployPreviewTTLForTest(defaultDeployPreviewTTL)
	_ = SetDeployPreviewRandForTest(nil)
	resetDeployPreviewTokens()
	startToolProcess = defaultStartToolProcess
	validateGitConnectionForDesktop = defaultValidateGitConnectionForDesktop
	cloneGitRepoForDesktop = defaultCloneGitRepoForDesktop
	openExternalURLForDesktop = defaultOpenExternalURLForDesktop
	inspectDBContainerEnvForDesktop = defaultInspectDBContainerEnvForDesktop
	publishedDBContainerPortForDesktop = defaultPublishedDBContainerPortForDesktop
	inspectDBContainerIPsForDesktop = defaultInspectDBContainerIPsForDesktop
	probeReachableDBHostForDesktop = defaultProbeReachableDBHostForDesktop
	runDBQueryForDesktop = defaultRunDBQueryForDesktop
	estimateDBSizeForDesktop = defaultEstimateDBSizeForDesktop
	freeDiskSpaceForDesktop = defaultFreeDiskSpaceForDesktop
	openDBDumpReaderForDesktop = defaultOpenDBDumpReaderForDesktop
	openDBImportWriterForDesktop = defaultOpenDBImportWriterForDesktop
	createDBDumpDestForDesktop = defaultCreateDBDumpDestForDesktop
	resetDBTransferGens()
	runGlobalServicesComposeForDesktop = defaultRunGlobalServicesComposeForDesktop
	ensureGlobalServicesForDesktop = defaultEnsureGlobalServicesForDesktop
	runHostPortProbeForDesktop = defaultRunHostPortProbeForDesktop
	chooseSaveFileForDesktop = defaultChooseSaveFileForDesktop
	writeLogFileForDesktop = defaultWriteLogFileForDesktop
	chooseOpenFileForDesktop = defaultChooseOpenFileForDesktop
	resolveSnapshotContextForDesktop = defaultResolveSnapshotContextForDesktop
	listSnapshotsForDesktop = defaultListSnapshotsForDesktop
	createSnapshotForDesktop = defaultCreateSnapshotForDesktop
	restoreSnapshotForDesktop = defaultRestoreSnapshotForDesktop
	exportSnapshotForDesktop = defaultExportSnapshotForDesktop
	registerSnapshotOpForDesktop = RegisterOp
	snapshotRestoreNow = time.Now
	resetSnapshotRestorePreviews()
	resetSnapshotOpGens()
}

// GovardCommandTimeoutForTest exposes the CLI timeout policy for tests.
func GovardCommandTimeoutForTest(args []string) time.Duration {
	return govardCommandTimeout(args)
}

// NewGovardCommandTimeoutErrorForTest exposes the timeout error builder.
func NewGovardCommandTimeoutErrorForTest(args []string, timeout time.Duration, output string) error {
	return newGovardCommandTimeoutError(args, timeout, output)
}

// BuildGitConnectionHelpForTest exposes the git failure help builder.
func BuildGitConnectionHelpForTest(protocol string, repoURL string, cause error) string {
	return buildGitConnectionHelp(protocol, repoURL, cause)
}

// SimulateGovardCommandTimeoutForTest returns the exact error the desktop
// runner produces when the given args hit their deadline, without spawning
// a process.
func SimulateGovardCommandTimeoutForTest(args []string) error {
	return newGovardCommandTimeoutError(args, govardCommandTimeout(args), "")
}

// RunRemoteSyncBackgroundWithOptionsForTest exposes the background sync
// starter with an injected platform so tests capture its events.
func RunRemoteSyncBackgroundWithOptionsForTest(
	ctx context.Context,
	p Platform,
	project string,
	remoteName string,
	preset string,
	options map[string]bool,
) error {
	return runRemoteSyncBackgroundWithOptions(ctx, p, project, remoteName, preset, options)
}

// FallbackServiceNamesForTest exposes the label-less parser vocabulary.
func FallbackServiceNamesForTest() []string {
	return append([]string{}, fallbackServiceNames...)
}

// ExtractProjectAndServiceForTest exposes label-less container parsing.
func ExtractProjectAndServiceForTest(names []string) (string, string) {
	return extractProjectAndService(container.Summary{Names: names})
}

// ValidateSafeCloneDestinationForTest exposes clone destination safety.
func ValidateSafeCloneDestinationForTest(cleanPath string) error {
	return validateSafeCloneDestination(cleanPath)
}

// NewLockedDeleteBufferForTest builds the delete-output buffer shared by
// both engine streams in a deletion.
func NewLockedDeleteBufferForTest(limit int) (io.Writer, func() string) {
	buf := newLockedDeleteBuffer(limit)
	return buf, buf.String
}

// AppendDeleteWarningsForTest exposes delete-warning extraction.
func AppendDeleteWarningsForTest(base, output string) string {
	return appendDeleteWarnings(base, output)
}

// SetRegistryFanoutLimitForTest overrides the registry fan-out bound so
// starvation tests run single-file.
func SetRegistryFanoutLimitForTest(limit int) func() {
	previous := registryEnvFanoutLimitValue
	registryEnvFanoutLimitValue = limit
	return func() {
		registryEnvFanoutLimitValue = previous
	}
}

// SetBuildRegistryEnvironmentEntryForTest overrides the entry renderer so
// panic recovery tests can simulate a corrupt entry.
func SetBuildRegistryEnvironmentEntryForTest(fn func(engine.ProjectRegistryEntry, string) Environment) func() {
	previous := buildRegistryEnvironmentEntryFunc
	buildRegistryEnvironmentEntryFunc = fn
	return func() {
		buildRegistryEnvironmentEntryFunc = previous
	}
}

// BuildRegistryEnvironmentEntrySafeForTest exposes the recovering renderer.
func BuildRegistryEnvironmentEntrySafeForTest(ent engine.ProjectRegistryEntry, name string) (Environment, string) {
	return buildRegistryEnvironmentEntrySafe(ent, name)
}

// BuildRegistryEnvironmentEntryForTest exposes the registry entry renderer.
func BuildRegistryEnvironmentEntryForTest(ent engine.ProjectRegistryEntry, name string) Environment {
	return buildRegistryEnvironmentEntry(ent, name)
}

// IsTrayRunningStatusForTest exposes the tray running predicate.
func IsTrayRunningStatusForTest(status string) bool {
	return isTrayRunningStatus(status)
}

// IsActiveEnvironmentStatusForTest exposes the active-counter predicate.
func IsActiveEnvironmentStatusForTest(status string) bool {
	return isActiveEnvironmentStatus(status)
}

// SetLogStreamDrainTimeoutForTest overrides the scanner drain bound so
// stuck-scanner tests run in milliseconds.
func SetLogStreamDrainTimeoutForTest(timeout time.Duration) func() {
	previous := logStreamDrainTimeout
	logStreamDrainTimeout = timeout
	return func() {
		logStreamDrainTimeout = previous
	}
}

// ReapLogStreamProcessForTest exposes the log-stream reaper.
func ReapLogStreamProcessForTest(cmd *exec.Cmd, done <-chan struct{}) {
	reapLogStreamProcess(cmd, done)
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

// SanitizeUITextForTest exposes UI-bound text sanitization for tests.
func SanitizeUITextForTest(s string, max int) string {
	return sanitizeUIText(s, max)
}

// ResolveLogContainerForTest exposes log target container resolution.
func ResolveLogContainerForTest(name string, services []string) string {
	info := &projectInfo{name: name, services: map[string]bool{}}
	for _, service := range services {
		if trimmed := strings.TrimSpace(service); trimmed != "" {
			info.services[trimmed] = true
		}
	}
	return resolveLogContainer(info, "")
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

// ScrubDBClientURLSecretForTest exposes DB password scrubbing for errors.
func ScrubDBClientURLSecretForTest(text, user, password string) string {
	return scrubDBClientURLSecret(text, user, password)
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

// RollupEnvironmentStatusForTest exposes the container-count to environment-status rollup for tests.
func RollupEnvironmentStatusForTest(runningCount int, totalCount int) string {
	return rollupEnvironmentStatus(runningCount, totalCount)
}

// WithCommandOutputForTest exposes the base-message plus capped CLI output builder for tests.
func WithCommandOutputForTest(base string, commandOutput string) string {
	return withCommandOutput(base, commandOutput)
}

// ComposeProjectNameForToggleForTest exposes the UI identifier to compose project name mapping for tests.
func ComposeProjectNameForToggleForTest(project string) string {
	return composeProjectNameForToggle(project)
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
	projectRoot string,
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
		projectRoot,
	)
	if err != nil {
		return "", err
	}

	return resolved, nil
}

// ResolveRemoteConfigForCapabilityForTest exposes the desktop's remote
// resolution, including its synthetic branch, so a test can assert the identity
// an action would be given — and be refused when there is none.
func ResolveRemoteConfigForCapabilityForTest(
	cfg engine.Config,
	requestedRemoteName string,
	capability string,
	projectRoot string,
) (string, engine.RemoteConfig, error) {
	return resolveRemoteConfigForCapability(cfg, requestedRemoteName, capability, projectRoot)
}

// ListProjectRemotesByPathForTest exposes the desktop's remote listing for a
// project root.
func ListProjectRemotesByPathForTest(root string) (RemoteSnapshot, error) {
	return listProjectRemotesByPath(root)
}

// StubResolveSandboxRemoteForDesktopForTest replaces the desktop's synthetic
// resolution seam for the duration of a test and returns a func that restores
// the real one, so listing and opening a sandbox never reach a Docker daemon.
func StubResolveSandboxRemoteForDesktopForTest(
	fn func(ctx context.Context, cfg engine.Config, projectRoot string) (engine.RemoteConfig, deploy.SandboxLiveness, error),
) func() {
	previous := resolveSandboxRemoteForDesktop
	if fn == nil {
		resolveSandboxRemoteForDesktop = deploy.ResolveSandboxRemoteForConfig
	} else {
		resolveSandboxRemoteForDesktop = fn
	}
	return func() {
		resolveSandboxRemoteForDesktop = previous
	}
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

// SetDesktopGitTimeoutsForTest overrides the git ls-remote and clone
// ceilings so timeout tests run in milliseconds, not minutes.
func SetDesktopGitTimeoutsForTest(validate, clone time.Duration) func() {
	previousValidate, previousClone := desktopGitValidateTimeout, desktopGitCloneTimeout
	desktopGitValidateTimeout, desktopGitCloneTimeout = validate, clone
	return func() {
		desktopGitValidateTimeout, desktopGitCloneTimeout = previousValidate, previousClone
	}
}

// DefaultValidateGitConnectionForTest runs the production git validator,
// bypassing any test override, so timeout tests exercise the real subprocess.
func DefaultValidateGitConnectionForTest(protocol string, repoURL string) error {
	return defaultValidateGitConnectionForDesktop(protocol, repoURL)
}

// DefaultCloneGitRepoForTest runs the production git clone, bypassing any
// test override, so timeout tests exercise the real subprocess.
func DefaultCloneGitRepoForTest(repoURL string, destination string) error {
	return defaultCloneGitRepoForDesktop(repoURL, destination)
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

// SetChooseSaveFileForDesktopForTest overrides the desktop save file picker for tests.
func SetChooseSaveFileForDesktopForTest(
	fn func(p Platform, opts SaveFileOptions) (string, error),
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

// AppPlatformsForTest returns the platform held by the App and each service,
// in the order App, Settings, Onboarding, Environment, Remote, System, Logs,
// Global, Update, Deploy, Operations, Health, Tool, DB, Snapshot.
func AppPlatformsForTest(app *App) []Platform {
	return []Platform{
		app.platform,
		app.Settings.platform,
		app.Onboarding.platform,
		app.Environment.platform,
		app.Remote.platform,
		app.System.platform,
		app.Logs.platform,
		app.Global.platform,
		app.Update.platform,
		app.Deploy.platform,
		app.Operations.platform,
		app.Health.platform,
		app.Tool.platform,
		app.DB.platform,
		app.Snapshot.platform,
	}
}

// DecideCloseForTest exposes the close decision for tests.
func DecideCloseForTest(runInBackground, trayAvailable bool) CloseAction {
	return decideClose(runInBackground, trayAvailable)
}

// NewCloseGateForTest builds a close gate for tests.
func NewCloseGateForTest() *closeGate { return &closeGate{} }

// BuildTrayProjectsForTest exposes the tray menu model for tests.
func BuildTrayProjectsForTest(d Dashboard) []trayProject { return buildTrayProjects(d) }

// TrayTitleForTest exposes the tray label with its running-op count for tests.
func TrayTitleForTest(runningOps int) string { return trayTitle(runningOps) }

// OperationNotifyCursorPathForTest exposes the persisted notification cursor
// location for tests.
func OperationNotifyCursorPathForTest() string { return operationNotifyCursorPath() }

// PollOperationNotificationsForTest runs one watcher iteration (collect,
// emit, then persist) and reports the next cursor, so tests can drive crash
// orderings deterministically without sleeping out the poll interval.
func PollOperationNotificationsForTest(p Platform, cursor string) string {
	return pollOperationNotifications(p, nil, cursor)
}

// WriteDesktopOperationEventForTest exposes the desktop operation-event
// writer for tests, so the stored engine message scrub is pinnable without
// driving a full service call. Tests only.
func WriteDesktopOperationEventForTest(
	operation string,
	status engine.OperationStatus,
	project string,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) {
	writeDesktopOperationEvent(operation, status, project, source, destination, message, category, duration)
}

// ServiceContextsForTest returns each bound service's lifecycle context.
func ServiceContextsForTest(app *App) map[string]context.Context {
	return map[string]context.Context{
		"Settings": app.Settings.lifecycleContext(), "Onboarding": app.Onboarding.lifecycleContext(),
		"Environment": app.Environment.lifecycleContext(), "Remote": app.Remote.lifecycleContext(),
		"System": app.System.lifecycleContext(), "Logs": app.Logs.lifecycleContext(),
		"Global": app.Global.lifecycleContext(), "Update": app.Update.lifecycleContext(),
	}
}

// StartOperationWatcherForTest starts the operation notification watcher the
// way Wails startup would, for tests that need it running.
func StartOperationWatcherForTest(app *App, ctx context.Context) {
	app.watcher.start(ctx)
}

// StopOperationWatcherForTest stops it again.
func StopOperationWatcherForTest(app *App) {
	app.watcher.stop()
}

// SetOperationWatcherRefreshForTest installs the hook the tray uses to rebuild
// its menu, so a test can count refreshes without a tray.
func SetOperationWatcherRefreshForTest(app *App, refresh func()) {
	app.watcher.onEvent = refresh
}

// SetOperationWatcherPollIntervalForTest overrides how often the operation
// watcher polls. Must be called before StartOperationWatcherForTest.
//
// The production cadence is two seconds, which is right for a tray and wrong for
// a test: a watcher test would otherwise have to sleep past a real interval to
// observe one poll. Shortening it keeps the test asserting the thing that
// actually matters -- that a burst of events costs one refresh, not one per
// event -- without the suite paying production cadence for it.
func SetOperationWatcherPollIntervalForTest(app *App, interval time.Duration) {
	app.watcher.pollInterval = interval
}

// DefaultPlatformForTest returns the build-tag-selected default platform.
func DefaultPlatformForTest() Platform {
	return newDefaultPlatform()
}

// MapSyncProgressLineForTest exposes the sync log line to structured progress
// mapping for tests.
func MapSyncProgressLineForTest(line string) (SyncProgressPayload, bool) {
	return mapSyncProgressLine(line)
}

// ScanSyncPipeForTest runs scanSyncPipe to completion against r.
func ScanSyncPipeForTest(p Platform, r io.Reader) {
	done := make(chan struct{}, 1)
	scanSyncPipe(context.Background(), p, r, done)
	<-done
}

// SyncFailedMessageForTest exposes sync failure payload formatting for tests.
func SyncFailedMessageForTest(err error) string {
	return syncFailedMessage(err)
}

// HumanOpMessageForTest exposes the human operation-failure catalog for tests.
func HumanOpMessageForTest(op string, err error) OpMessage {
	return humanOpMessage(op, err)
}

// ScanLogPipeForTest runs scanLogPipe to completion against r.
func ScanLogPipeForTest(p Platform, r io.Reader, event string) {
	done := make(chan struct{}, 1)
	scanLogPipe(context.Background(), p, r, event, done)
	<-done
}

// BackgroundSyncIdleTimeoutForTest exposes backgroundSyncIdleTimeout for tests in /tests.
func BackgroundSyncIdleTimeoutForTest() time.Duration {
	return backgroundSyncIdleTimeout
}

// SetBackgroundSyncIdleTimeoutForTest overrides the idle ceiling and
// returns a restore func for defer.
func SetBackgroundSyncIdleTimeoutForTest(d time.Duration) func() {
	previous := backgroundSyncIdleTimeout
	backgroundSyncIdleTimeout = d
	return func() {
		backgroundSyncIdleTimeout = previous
	}
}

// BrandTrayForTest exposes brandTray for tests in /tests.
func BrandTrayForTest(t interface {
	SetLabel(label string)
	SetTooltip(tooltip string)
}) {
	brandTray(t)
}

// ResetOperationRegistryForTest clears the cancellable-op registry so
// registry tests stay hermetic. The registry API itself (RegisterOp,
// CancelOp, FinishOp, ListOps) is exported, so no other seams are needed.
func ResetOperationRegistryForTest() {
	resetOperationRegistry()
}

// AgeDoctorPreviewTokensForTest advances every stored preview token's age
// by the given duration, so expiry tests pin the TTL refusal without
// sleeping out the real 15-minute window.
func AgeDoctorPreviewTokensForTest(age time.Duration) {
	doctorPreviewTokens.Lock()
	defer doctorPreviewTokens.Unlock()
	for id, tok := range doctorPreviewTokens.byID {
		tok.createdAt = tok.createdAt.Add(-age)
		doctorPreviewTokens.byID[id] = tok
	}
}

// SetDoctorRunDiagnosticsForDesktopForTest overrides the doctor engine run
// behind RunDoctor for the duration of a test and returns a func that
// restores the real one, so doctor service tests never probe the host.
func SetDoctorRunDiagnosticsForDesktopForTest(fn func() engine.DoctorReport) func() {
	previous := doctorRunDiagnosticsForDesktop
	if fn == nil {
		doctorRunDiagnosticsForDesktop = func() engine.DoctorReport {
			return engine.RunDoctorDiagnostics(engine.DoctorDependencies{})
		}
	} else {
		doctorRunDiagnosticsForDesktop = fn
	}
	return func() {
		doctorRunDiagnosticsForDesktop = previous
	}
}

// SetDoctorFixExecutorForDesktopForTest overrides the per-check fix
// executor behind Preview/ApplyDoctorFix for the duration of a test and
// returns a func that restores the real one, so tests never mutate the
// host (no mkdir, no docker, no config writes).
func SetDoctorFixExecutorForDesktopForTest(fn DoctorFixExecutor) func() {
	previous := doctorFixExecutorForDesktop
	if fn == nil {
		doctorFixExecutorForDesktop = defaultDoctorFixExecutor
	} else {
		doctorFixExecutorForDesktop = fn
	}
	return func() {
		doctorFixExecutorForDesktop = previous
	}
}

// ResetDoctorServiceForTest clears single-use preview tokens so doctor
// service tests stay hermetic.
func ResetDoctorServiceForTest() {
	resetDoctorPreviewTokens()
}

// SetDoctorRandReadForDesktopForTest overrides the randomness source behind
// preview-token minting for the duration of a test and returns a func that
// restores crypto/rand, so minting failures are testable deterministically.
func SetDoctorRandReadForDesktopForTest(fn func([]byte) (int, error)) func() {
	previous := doctorRandRead
	if fn == nil {
		doctorRandRead = rand.Read
	} else {
		doctorRandRead = fn
	}
	return func() {
		doctorRandRead = previous
	}
}

// ResetAuditServiceForTest clears the last-result-per-project index so audit
// service tests stay hermetic.
func ResetAuditServiceForTest() {
	resetLastAuditResults()
}

// SetAuditResolveForDesktopForTest overrides integrity target resolution
// behind RunIntegrityAudit for the duration of a test and returns a func
// that restores the real one, so audit service tests never probe the host
// (no framework detection, no git, no config reads).
func SetAuditResolveForDesktopForTest(fn func(projectRoot string) (audit.RunRequest, error)) func() {
	previous := resolveAuditRunRequestForDesktop
	if fn == nil {
		resolveAuditRunRequestForDesktop = defaultResolveAuditRunRequest
	} else {
		resolveAuditRunRequestForDesktop = fn
	}
	return func() {
		resolveAuditRunRequestForDesktop = previous
	}
}

// SetAuditRunnerForDesktopForTest overrides audit runner execution behind
// RunIntegrityAudit for the duration of a test and returns a func that
// restores the real one, so tests never run real analyzers.
func SetAuditRunnerForDesktopForTest(fn func(ctx context.Context, request audit.RunRequest) (audit.RunResult, error)) func() {
	previous := runAuditRunnerForDesktop
	if fn == nil {
		runAuditRunnerForDesktop = defaultRunAuditRunner
	} else {
		runAuditRunnerForDesktop = fn
	}
	return func() {
		runAuditRunnerForDesktop = previous
	}
}

// SetAuditResultReaderForDesktopForTest overrides persisted-run reads behind
// GetAuditResult for the duration of a test and returns a func that restores
// the real store-backed one.
func SetAuditResultReaderForDesktopForTest(fn func(projectID, sessionID, runID string) (audit.RunResult, error)) func() {
	previous := readAuditRunResultForDesktop
	if fn == nil {
		readAuditRunResultForDesktop = defaultReadAuditRunResult
	} else {
		readAuditRunResultForDesktop = fn
	}
	return func() {
		readAuditRunResultForDesktop = previous
	}
}

// SetAuditRunListForDesktopForTest overrides the session run listing behind
// GetAuditDiff for the duration of a test and returns a func that restores
// the real store-backed one.
func SetAuditRunListForDesktopForTest(fn func(projectID, sessionID string) ([]string, error)) func() {
	previous := listAuditRunIDsForDesktop
	if fn == nil {
		listAuditRunIDsForDesktop = defaultListAuditRunIDs
	} else {
		listAuditRunIDsForDesktop = fn
	}
	return func() {
		listAuditRunIDsForDesktop = previous
	}
}

// SetAuditPreRecordHookForDesktopForTest installs a hook that RunIntegrityAudit
// runs synchronously between its completion check and the banner record (nil
// in production). Tests use it to deliver CancelAudit deterministically inside
// that race window without sleeps, proving late completions are discarded.
func SetAuditPreRecordHookForDesktopForTest(fn func(opID string)) func() {
	previous := auditPreRecordHookForDesktop
	auditPreRecordHookForDesktop = fn
	return func() {
		auditPreRecordHookForDesktop = previous
	}
}

// AuditCancelledSetSizeForTest reports how many cancellation marks the banner
// index currently holds, so tests can prove the set stays bounded.
func AuditCancelledSetSizeForTest() int {
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	return len(lastAuditResultsCache.cancelled)
}

// PlantAuditCancelledMarksForTest inserts stale marks directly (bypassing the
// insert purge), simulating paths that predate it: unacked ones must be
// purged on the next insert, won ones must survive. Age is measured back from
// now.
func PlantAuditCancelledMarksForTest(unacked, won int, age time.Duration) {
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	at := time.Now().Add(-age)
	for i := 0; i < unacked; i++ {
		lastAuditResultsCache.cancelled["audit:stale-unacked-"+strconv.Itoa(i)] = auditCancelledMark{at: at}
	}
	for i := 0; i < won; i++ {
		lastAuditResultsCache.cancelled["audit:stale-won-"+strconv.Itoa(i)] = auditCancelledMark{won: true, at: at}
	}
}

// PlantAuditCancelledMarkForOpForTest inserts one mark for an explicit op id
// (bypassing the insert purge), so tests can arm a stale won mark for exactly
// the op under test.
func PlantAuditCancelledMarkForOpForTest(opID string, won bool, age time.Duration) {
	if opID == "" {
		return
	}
	lastAuditResultsCache.Lock()
	defer lastAuditResultsCache.Unlock()
	lastAuditResultsCache.cancelled[opID] = auditCancelledMark{won: won, at: time.Now().Add(-age)}
}

// SetStartToolProcessForTest replaces the tool-run process spawner for the
// duration of a test and returns a func that restores the real one, so tool
// runs never spawn project binaries under test.
func SetStartToolProcessForTest(fn func(ctx context.Context, dir string, argv []string) (io.Reader, io.Reader, func() error, error)) func() {
	previous := startToolProcess
	if fn == nil {
		startToolProcess = defaultStartToolProcess
	} else {
		startToolProcess = fn
	}
	return func() {
		startToolProcess = previous
	}
}

// ToolRunTimeoutForTest exposes the tool-run ceiling for tests.
func ToolRunTimeoutForTest(presetID string) time.Duration {
	return toolRunTimeout(presetID)
}

// ScrubToolHistoryArgsForTest exposes history arg scrubbing for tests.
func ScrubToolHistoryArgsForTest(args map[string]string) map[string]string {
	return scrubToolHistoryArgs(args)
}

// AppendToolHistoryLineForTest appends one history entry for a project with
// the production single-write path, so tests can drive the store with arg
// shapes (e.g. secret key names) the preset validator would reject at the
// RunTool gate.
func AppendToolHistoryLineForTest(project string, entry ToolHistoryEntry) error {
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return err
	}
	return appendToolHistoryEntryForRoot(root, entry)
}

// DBWriteConfirmTokenForTest exposes the per-project write confirm token the
// data tab confirm dialog passes back for a write statement.
func DBWriteConfirmTokenForTest(project string) string {
	return dbWriteConfirmToken(project)
}

// ClassifyDBStatementForTest exposes the read-only statement classifier:
// the first keyword plus the single-statement rule.
func ClassifyDBStatementForTest(sql string) (string, bool, error) {
	return classifyDBStatement(sql)
}

// SetInspectDBContainerEnvForDesktopForTest overrides docker inspect env
// sampling in DB discovery.
func SetInspectDBContainerEnvForDesktopForTest(
	fn func(container string) (string, error),
) func() {
	previous := inspectDBContainerEnvForDesktop
	if fn == nil {
		inspectDBContainerEnvForDesktop = defaultInspectDBContainerEnvForDesktop
	} else {
		inspectDBContainerEnvForDesktop = fn
	}
	return func() {
		inspectDBContainerEnvForDesktop = previous
	}
}

// SetPublishedDBContainerPortForDesktopForTest overrides docker port sampling
// in DB discovery.
func SetPublishedDBContainerPortForDesktopForTest(
	fn func(container string, internalPort string) (string, error),
) func() {
	previous := publishedDBContainerPortForDesktop
	if fn == nil {
		publishedDBContainerPortForDesktop = defaultPublishedDBContainerPortForDesktop
	} else {
		publishedDBContainerPortForDesktop = fn
	}
	return func() {
		publishedDBContainerPortForDesktop = previous
	}
}

// SetInspectDBContainerIPsForDesktopForTest overrides docker inspect IP
// sampling in DB discovery.
func SetInspectDBContainerIPsForDesktopForTest(
	fn func(container string) (string, error),
) func() {
	previous := inspectDBContainerIPsForDesktop
	if fn == nil {
		inspectDBContainerIPsForDesktop = defaultInspectDBContainerIPsForDesktop
	} else {
		inspectDBContainerIPsForDesktop = fn
	}
	return func() {
		inspectDBContainerIPsForDesktop = previous
	}
}

// SetProbeReachableDBHostForDesktopForTest overrides container-IP reachability
// probing in DB discovery (the production probe dials TCP).
func SetProbeReachableDBHostForDesktopForTest(
	fn func(candidates []string, port string) string,
) func() {
	previous := probeReachableDBHostForDesktop
	if fn == nil {
		probeReachableDBHostForDesktop = defaultProbeReachableDBHostForDesktop
	} else {
		probeReachableDBHostForDesktop = fn
	}
	return func() {
		probeReachableDBHostForDesktop = previous
	}
}

// SetEstimateDBSizeForDesktopForTest overrides the dump size estimator. The
// recording target carries no password by construction.
func SetEstimateDBSizeForDesktopForTest(fn func(target DBTransferTarget) (int64, error)) func() {
	previous := estimateDBSizeForDesktop
	if fn == nil {
		estimateDBSizeForDesktop = defaultEstimateDBSizeForDesktop
	} else {
		estimateDBSizeForDesktop = func(_ context.Context, conn dbConnection) (int64, error) {
			return fn(conn.transferTarget())
		}
	}
	return func() {
		estimateDBSizeForDesktop = previous
	}
}

// SetFreeDiskSpaceForDesktopForTest overrides the destination free-space probe.
func SetFreeDiskSpaceForDesktopForTest(fn func(path string) (uint64, error)) func() {
	previous := freeDiskSpaceForDesktop
	if fn == nil {
		freeDiskSpaceForDesktop = defaultFreeDiskSpaceForDesktop
	} else {
		freeDiskSpaceForDesktop = fn
	}
	return func() {
		freeDiskSpaceForDesktop = previous
	}
}

// SetOpenDBDumpReaderForDesktopForTest overrides dump-side process startup.
// The recording target carries no password by construction.
func SetOpenDBDumpReaderForDesktopForTest(
	fn func(ctx context.Context, target DBTransferTarget) (io.ReadCloser, func() error, error),
) func() {
	previous := openDBDumpReaderForDesktop
	if fn == nil {
		openDBDumpReaderForDesktop = defaultOpenDBDumpReaderForDesktop
	} else {
		openDBDumpReaderForDesktop = func(ctx context.Context, conn dbConnection) (io.ReadCloser, func() error, error) {
			return fn(ctx, conn.transferTarget())
		}
	}
	return func() {
		openDBDumpReaderForDesktop = previous
	}
}

// SetOpenDBImportWriterForDesktopForTest overrides import-side process
// startup. The recording target carries no password by construction.
func SetOpenDBImportWriterForDesktopForTest(
	fn func(ctx context.Context, target DBTransferTarget) (io.WriteCloser, func() error, error),
) func() {
	previous := openDBImportWriterForDesktop
	if fn == nil {
		openDBImportWriterForDesktop = defaultOpenDBImportWriterForDesktop
	} else {
		openDBImportWriterForDesktop = func(ctx context.Context, conn dbConnection) (io.WriteCloser, func() error, error) {
			return fn(ctx, conn.transferTarget())
		}
	}
	return func() {
		openDBImportWriterForDesktop = previous
	}
}

// SetCreateDBDumpDestForDesktopForTest overrides the dump destination file
// creator, so a test can inject a failing writer and pin mid-stream cleanup.
func SetCreateDBDumpDestForDesktopForTest(fn func(path string) (io.WriteCloser, error)) func() {
	previous := createDBDumpDestForDesktop
	if fn == nil {
		createDBDumpDestForDesktop = defaultCreateDBDumpDestForDesktop
	} else {
		createDBDumpDestForDesktop = fn
	}
	return func() {
		createDBDumpDestForDesktop = previous
	}
}

// CreateDBDumpDestForTest exposes the dump destination file creator for
// tests (mode assertions run it unstubbed under a hostile umask).
func CreateDBDumpDestForTest(path string) (io.WriteCloser, error) {
	return createDBDumpDestForDesktop(path)
}

// FormatBytesForTest exposes preflight byte formatting for tests.
func FormatBytesForTest(bytes int64) string {
	return formatDBBytes(bytes)
}

// SniffDBDumpHeaderForTest exposes dump header engine detection for tests.
func SniffDBDumpHeaderForTest(head []byte) (string, bool) {
	return sniffDBDumpHead(head)
}

// BuildDBTransferDockerArgsForTest exposes the secret-free docker argv
// builder: the password must appear in the env file, never in argv.
func BuildDBTransferDockerArgsForTest(
	container string,
	password string,
	envName string,
	script string,
) ([]string, string, func(), error) {
	return buildDBTransferDockerArgs(container, password, envName, script)
}

// BuildDBQueryDockerArgsForTest exposes the ad-hoc query docker argv
// builder: the password must appear in the env file, never in argv.
func BuildDBQueryDockerArgsForTest(
	container string,
	scheme string,
	user string,
	password string,
	sql string,
) ([]string, string, func(), error) {
	return buildDBQueryDockerArgs(dbConnection{
		container: container,
		scheme:    scheme,
		user:      user,
		password:  password,
		database:  "appdb",
	}, sql)
}

// SniffDBDumpFileForTest exposes the call-time import header check for tests.
func SniffDBDumpFileForTest(path string) (string, bool, error) {
	return sniffDBDumpFile(path)
}

// RevalidateDBImportSourceForTest exposes the op-start import re-validation
// (re-sniff plus fresh stat from the opened file) for tests.
func RevalidateDBImportSourceForTest(path string, scheme string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = file.Close() }()
	return revalidateDBImportSourceFile(file, scheme)
}

// SetRunDBQueryForDesktopForTest overrides ad-hoc query execution. The
// recording call carries no password by construction.
func SetRunDBQueryForDesktopForTest(fn func(call DBQueryCall) (string, error)) func() {
	previous := runDBQueryForDesktop
	if fn == nil {
		runDBQueryForDesktop = defaultRunDBQueryForDesktop
	} else {
		runDBQueryForDesktop = func(_ context.Context, target dbConnection, sql string) (string, error) {
			return fn(DBQueryCall{
				Container: target.container,
				Scheme:    target.scheme,
				Host:      target.host,
				Port:      target.port,
				Database:  target.database,
				User:      target.user,
				SQL:       sql,
			})
		}
	}
	return func() {
		runDBQueryForDesktop = previous
	}
}

// SetResolveSnapshotContextForDesktopForTest overrides project-to-snapshot
// root plus config resolution.
func SetResolveSnapshotContextForDesktopForTest(
	fn func(project string) (string, engine.Config, error),
) func() {
	previous := resolveSnapshotContextForDesktop
	if fn == nil {
		resolveSnapshotContextForDesktop = defaultResolveSnapshotContextForDesktop
	} else {
		resolveSnapshotContextForDesktop = fn
	}
	return func() {
		resolveSnapshotContextForDesktop = previous
	}
}

// SetListSnapshotsForDesktopForTest overrides snapshot listing.
func SetListSnapshotsForDesktopForTest(
	fn func(root string) ([]engine.SnapshotMetadata, error),
) func() {
	previous := listSnapshotsForDesktop
	if fn == nil {
		listSnapshotsForDesktop = defaultListSnapshotsForDesktop
	} else {
		listSnapshotsForDesktop = fn
	}
	return func() {
		listSnapshotsForDesktop = previous
	}
}

// SetCreateSnapshotForDesktopForTest overrides snapshot creation. The func
// receives the op context so a test can prove cancellation reaches the
// engine call instead of being dropped at the seam.
func SetCreateSnapshotForDesktopForTest(
	fn func(ctx context.Context, root string, cfg engine.Config, name string) (string, error),
) func() {
	previous := createSnapshotForDesktop
	if fn == nil {
		createSnapshotForDesktop = defaultCreateSnapshotForDesktop
	} else {
		createSnapshotForDesktop = fn
	}
	return func() {
		createSnapshotForDesktop = previous
	}
}

// SetRestoreSnapshotForDesktopForTest overrides snapshot restore. The
// recording target carries no secrets by construction.
func SetRestoreSnapshotForDesktopForTest(
	fn func(ctx context.Context, target SnapshotRestoreTarget) error,
) func() {
	previous := restoreSnapshotForDesktop
	if fn == nil {
		restoreSnapshotForDesktop = defaultRestoreSnapshotForDesktop
	} else {
		restoreSnapshotForDesktop = fn
	}
	return func() {
		restoreSnapshotForDesktop = previous
	}
}

// SetExportSnapshotForDesktopForTest overrides snapshot export. The func
// receives the op context so a test can prove cancellation reaches the
// engine call instead of being dropped at the seam.
func SetExportSnapshotForDesktopForTest(
	fn func(ctx context.Context, root string, name string, target string) error,
) func() {
	previous := exportSnapshotForDesktop
	if fn == nil {
		exportSnapshotForDesktop = defaultExportSnapshotForDesktop
	} else {
		exportSnapshotForDesktop = fn
	}
	return func() {
		exportSnapshotForDesktop = previous
	}
}

// SetRegisterSnapshotOpForDesktopForTest overrides op registration for
// snapshot ops, so a test can force the contention path and prove a refused
// registration consumes no preview token.
func SetRegisterSnapshotOpForDesktopForTest(
	fn func(id string, project string, kind string, cancel context.CancelFunc) bool,
) func() {
	previous := registerSnapshotOpForDesktop
	if fn == nil {
		registerSnapshotOpForDesktop = RegisterOp
	} else {
		registerSnapshotOpForDesktop = fn
	}
	return func() {
		registerSnapshotOpForDesktop = previous
	}
}

// SetSnapshotRestoreNowForTest overrides the preview-token clock, so a test
// can age a token past its TTL without sleeping.
func SetSnapshotRestoreNowForTest(fn func() time.Time) func() {
	previous := snapshotRestoreNow
	if fn == nil {
		snapshotRestoreNow = time.Now
	} else {
		snapshotRestoreNow = fn
	}
	return func() {
		snapshotRestoreNow = previous
	}
}

// SnapshotRestorePreviewCountForTest exposes the live preview-token count so
// a test can prove eviction bounds the map.
func SnapshotRestorePreviewCountForTest() int {
	snapshotRestorePreviews.Lock()
	defer snapshotRestorePreviews.Unlock()
	return len(snapshotRestorePreviews.byToken)
}
