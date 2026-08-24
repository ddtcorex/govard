package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"govard/internal/engine"
)

const externalLintStopTimeout = 5 * time.Second

var (
	externalLintCleanupTimeout   = externalLintStopTimeout
	externalLintCleanupTimeoutMu sync.RWMutex
)

var lintIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var externalProviderIdentifier = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ExternalLintOptions describes an explicitly configured external provider.
// No provider is constructed unless the caller selects it.
type ExternalLintOptions struct {
	ID         string
	Config     engine.ExternalLintProviderConfig
	Docker     DockerClient
	RunCommand func(context.Context, string, []string, io.Writer, io.Writer) error
}

// ExternalLintProvider runs a user-configured Docker lint command. It is not
// a fallback for the Govard-owned backend.
type ExternalLintProvider struct {
	id     string
	config engine.ExternalLintProviderConfig
	docker DockerClient
	hooks  externalLintLifecycleHooks
}

type externalLintLifecycleHooks struct {
	mu                     sync.RWMutex
	beforeRun              func()
	afterRun               func()
	afterCancellationCheck func()
}

type externalLintLifecycleStage uint8

const (
	externalLintBeforeRun externalLintLifecycleStage = iota
	externalLintAfterRun
	externalLintAfterCancellationCheck
)

func NewExternalLintProvider(options ExternalLintOptions) (*ExternalLintProvider, error) {
	if err := validateExternalLintOptions(options.ID, options.Config); err != nil {
		return nil, err
	}
	docker := options.Docker
	if docker == nil {
		docker = NewExecDockerClient(options.RunCommand)
	}
	config := options.Config
	config.Command = append([]string(nil), options.Config.Command...)
	return &ExternalLintProvider{id: options.ID, config: config, docker: docker}, nil
}

func (provider *ExternalLintProvider) Name() string {
	if provider == nil {
		return ""
	}
	return provider.id
}

func (provider *ExternalLintProvider) Run(ctx context.Context, request LintRequest) (LintReport, error) {
	if provider == nil || provider.docker == nil {
		return LintReport{}, fmt.Errorf("external lint provider is not configured")
	}
	if ctx == nil {
		return LintReport{}, fmt.Errorf("external lint context is nil")
	}
	if request.Provider != provider.id {
		return LintReport{}, fmt.Errorf("external lint request provider %q does not match configured provider %q", request.Provider, provider.id)
	}
	if err := validateLintRequest(request); err != nil {
		return LintReport{}, err
	}
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return cancelledExternalLintResult(cancellationCause, nil)
	}
	if err := os.MkdirAll(request.RunDir, 0o755); err != nil {
		return LintReport{}, fmt.Errorf("create external lint run directory: %w", err)
	}
	if err := os.MkdirAll(request.CacheRoot, 0o755); err != nil {
		return LintReport{}, fmt.Errorf("create external lint cache directory: %w", err)
	}
	reportPath := filepath.Join(request.RunDir, "report.json")
	if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return LintReport{}, fmt.Errorf("remove prior external lint report: %w", err)
	}
	pullErr := provider.docker.Pull(ctx, provider.config.Image)
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return cancelledExternalLintResult(cancellationCause, labelExternalLintOperationError("pull external lint image", pullErr))
	}
	if pullErr != nil {
		return LintReport{}, fmt.Errorf("pull external lint image: %w", pullErr)
	}
	inspection, inspectErr := provider.docker.Inspect(ctx, provider.config.Image)
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return cancelledExternalLintResult(cancellationCause, labelExternalLintOperationError("inspect external lint image", inspectErr))
	}
	if inspectErr != nil {
		return LintReport{}, fmt.Errorf("inspect external lint image: %w", inspectErr)
	}
	imageDigest, err := immutableImageDigest(provider.config.Image, inspection)
	if err != nil {
		return LintReport{}, fmt.Errorf("inspect external lint image: %w", err)
	}
	toolchainDigest := provider.toolchainDigest(imageDigest, request)
	logFile, err := os.OpenFile(filepath.Join(request.RunDir, "external-lint.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return LintReport{}, fmt.Errorf("open external lint log: %w", err)
	}
	defer func() {
		if cerr := logFile.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "govard: close external lint log %s: %v\n", logFile.Name(), cerr)
		}
	}()
	container := ContainerRunRequest{
		Name:  containerName(request),
		Image: imageWithDigest(provider.config.Image, imageDigest),
		Args:  append([]string(nil), provider.config.Command...),
		Mounts: []ContainerMount{
			{Source: request.ProjectRoot, Target: "/source", ReadOnly: true},
			{Source: request.CacheRoot, Target: "/cache"},
			{Source: request.RunDir, Target: "/output"},
		},
		Environment: map[string]string{
			"GOVARD_LINT_IMAGE_DIGEST":     imageDigest,
			"GOVARD_LINT_TOOLCHAIN_DIGEST": toolchainDigest,
			"GOVARD_LINT_PROVIDER":         provider.id,
			"GOVARD_LINT_TARGET_ID":        request.TargetID,
			"GOVARD_LINT_TARGET_MODE":      string(request.Target.Mode),
			"GOVARD_LINT_TARGET_PATH":      request.Target.TargetPath,
			"GOVARD_LINT_PHP_VERSIONS":     strings.Join(request.PHPVersions(), ","),
		},
	}
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return cancelledExternalLintResult(cancellationCause, nil)
	}
	provider.runLifecycleHook(externalLintBeforeRun)
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return cancelledExternalLintResult(cancellationCause, nil)
	}
	output := io.Writer(logFile)
	if request.StreamWriter != nil {
		output = io.MultiWriter(logFile, request.StreamWriter)
	}
	commandErr := provider.runContainer(ctx, container, output)
	provider.runLifecycleHook(externalLintAfterRun)
	if cancellationCause := externalLintCancellationCause(ctx); cancellationCause != nil {
		return LintReport{Status: string(StatusCancelled)}, externalLintCancelledError(cancellationCause, provider.cleanupContainer(container.Name, logFile))
	}
	provider.runLifecycleHook(externalLintAfterCancellationCheck)
	exitCode, completedLintRun := lintExitCode(commandErr)
	if !completedLintRun {
		primary := fmt.Errorf("external lint container failed with infrastructure exit code %d", exitCode)
		return LintReport{}, withExternalLintCleanupError(primary, provider.removeCompletedContainer(container.Name, logFile))
	}
	report, err := readLintReport(reportPath)
	if err != nil {
		return LintReport{}, withExternalLintCleanupError(err, provider.removeCompletedContainer(container.Name, logFile))
	}
	if err := ValidateLintReport(request, report); err != nil {
		return LintReport{}, withExternalLintCleanupError(err, provider.removeCompletedContainer(container.Name, logFile))
	}
	if report.ImageDigest != imageDigest {
		return LintReport{}, withExternalLintCleanupError(fmt.Errorf("external lint report image digest does not match inspected image"), provider.removeCompletedContainer(container.Name, logFile))
	}
	if report.ToolchainDigest != toolchainDigest {
		return LintReport{}, withExternalLintCleanupError(fmt.Errorf("external lint report toolchain digest does not match configured command"), provider.removeCompletedContainer(container.Name, logFile))
	}
	if exitCode == 0 && report.Status != string(StatusPassed) {
		return LintReport{}, withExternalLintCleanupError(fmt.Errorf("successful external lint exit has non-passing report status"), provider.removeCompletedContainer(container.Name, logFile))
	}
	if exitCode == 1 && report.Status != string(StatusFailed) {
		return LintReport{}, withExternalLintCleanupError(fmt.Errorf("external lint findings exit has non-failed report status"), provider.removeCompletedContainer(container.Name, logFile))
	}
	_ = provider.removeCompletedContainer(container.Name, logFile)
	return report, nil
}

func (provider *ExternalLintProvider) toolchainDigest(imageDigest string, request LintRequest) string {
	return LintToolchainDigest(LintToolchain{
		Provider:         provider.id,
		Image:            imageDigest,
		Command:          provider.config.Command,
		PHPVersions:      request.PHPVersions(),
		Linters:          request.Profile.Linters,
		CodingStandard:   request.Profile.CodingStandard,
		PHPStanLevel:     request.Profile.PHPStanLevel,
		PHPStanExtension: request.Profile.PHPStanExtension,
	})
}

func (provider *ExternalLintProvider) runContainer(ctx context.Context, request ContainerRunRequest, output io.Writer) error {
	done := make(chan error, 1)
	go func() { done <- provider.docker.Run(ctx, request, output) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return externalLintCancellationCause(ctx)
	}
}

func (provider *ExternalLintProvider) cleanupContainer(name string, output io.Writer) error {
	stopErr := labelExternalLintCleanupError("stop external lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return provider.docker.Stop(ctx, name, externalLintStopTimeout)
	}))
	removeErr := labelExternalLintCleanupError("remove external lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return provider.docker.Remove(ctx, name)
	}))
	err := errors.Join(stopErr, removeErr)
	if err != nil && output != nil {
		_, _ = fmt.Fprintf(output, "external lint cleanup failed: %v\n", err)
	}
	return err
}

func (provider *ExternalLintProvider) removeCompletedContainer(name string, output io.Writer) error {
	err := labelExternalLintCleanupError("remove completed external lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return provider.docker.Remove(ctx, name)
	}))
	if err != nil && output != nil {
		_, _ = fmt.Fprintf(output, "remove completed external lint container failed: %v\n", err)
	}
	return err
}

func runBoundedDockerCleanup(run func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), externalLintCleanupDuration())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func externalLintCancelledError(cause, cleanupErr error) error {
	if cleanupErr != nil {
		return fmt.Errorf("external lint cancelled; cleanup: %w", errors.Join(cause, cleanupErr))
	}
	return fmt.Errorf("external lint cancelled: %w", cause)
}

func cancelledExternalLintResult(cause, operationErr error) (LintReport, error) {
	if operationErr != nil {
		return LintReport{Status: string(StatusCancelled)}, fmt.Errorf("external lint cancelled before container execution: %w", errors.Join(cause, operationErr))
	}
	return LintReport{Status: string(StatusCancelled)}, fmt.Errorf("external lint cancelled before container execution: %w", cause)
}

func externalLintCancellationCause(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

func labelExternalLintCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func labelExternalLintOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func withExternalLintCleanupError(primary, cleanupErr error) error {
	if cleanupErr == nil {
		return primary
	}
	return fmt.Errorf("external lint execution and cleanup failed: %w", errors.Join(primary, cleanupErr))
}

func externalLintCleanupDuration() time.Duration {
	externalLintCleanupTimeoutMu.RLock()
	defer externalLintCleanupTimeoutMu.RUnlock()
	return externalLintCleanupTimeout
}

// SetExternalLintCleanupTimeoutForTest temporarily adjusts the cancellation
// cleanup bound and returns a restoration function.
func SetExternalLintCleanupTimeoutForTest(timeout time.Duration) func() {
	externalLintCleanupTimeoutMu.Lock()
	previous := externalLintCleanupTimeout
	externalLintCleanupTimeout = timeout
	externalLintCleanupTimeoutMu.Unlock()
	return func() {
		externalLintCleanupTimeoutMu.Lock()
		externalLintCleanupTimeout = previous
		externalLintCleanupTimeoutMu.Unlock()
	}
}

// SetExternalLintLifecycleHooksForTest temporarily sets synchronization hooks
// for this provider only around the pre-run and completion boundaries.
func (provider *ExternalLintProvider) SetExternalLintLifecycleHooksForTest(beforeRun, afterRun, afterCancellationCheck func()) func() {
	if provider == nil {
		return func() {}
	}
	provider.hooks.mu.Lock()
	previousBeforeRun := provider.hooks.beforeRun
	previousAfterRun := provider.hooks.afterRun
	previousAfterCancellationCheck := provider.hooks.afterCancellationCheck
	provider.hooks.beforeRun = beforeRun
	provider.hooks.afterRun = afterRun
	provider.hooks.afterCancellationCheck = afterCancellationCheck
	provider.hooks.mu.Unlock()
	return func() {
		provider.hooks.mu.Lock()
		provider.hooks.beforeRun = previousBeforeRun
		provider.hooks.afterRun = previousAfterRun
		provider.hooks.afterCancellationCheck = previousAfterCancellationCheck
		provider.hooks.mu.Unlock()
	}
}

func (provider *ExternalLintProvider) runLifecycleHook(stage externalLintLifecycleStage) {
	provider.hooks.mu.RLock()
	configuredHook := provider.hooks.beforeRun
	switch stage {
	case externalLintAfterRun:
		configuredHook = provider.hooks.afterRun
	case externalLintAfterCancellationCheck:
		configuredHook = provider.hooks.afterCancellationCheck
	}
	provider.hooks.mu.RUnlock()
	if configuredHook != nil {
		configuredHook()
	}
}

func validateExternalLintOptions(id string, config engine.ExternalLintProviderConfig) error {
	if !externalProviderIdentifier.MatchString(id) {
		return fmt.Errorf("external lint provider ID %q is invalid", id)
	}
	if config.Type != "docker" {
		return fmt.Errorf("external lint provider %q has unsupported type %q", id, config.Type)
	}
	if strings.TrimSpace(config.Image) == "" {
		return fmt.Errorf("external lint provider %q is missing Docker image", id)
	}
	if len(config.Command) == 0 {
		return fmt.Errorf("external lint provider %q has an empty command", id)
	}
	for index, argument := range config.Command {
		if strings.TrimSpace(argument) == "" {
			return fmt.Errorf("external lint provider %q command argument %d is empty", id, index)
		}
	}
	return nil
}

func immutableImageDigest(image string, inspection ImageInspection) (string, error) {
	repository := imageRepository(image)
	for _, reference := range inspection.RepoDigests {
		if index := strings.LastIndex(reference, "@"); index >= 0 && imageRepository(reference[:index]) == repository && isImmutableDigest(reference[index+1:]) {
			return reference[index+1:], nil
		}
	}
	return "", fmt.Errorf("docker image has no immutable digest")
}

func lintExitCode(commandErr error) (int, bool) {
	if commandErr == nil {
		return 0, true
	}
	var exitErr *exec.ExitError
	if !errors.As(commandErr, &exitErr) {
		return -1, false
	}
	code := exitErr.ExitCode()
	return code, code == 1
}

func validateLintRequest(request LintRequest) error {
	if request.ProjectRoot == "" || request.RunDir == "" || request.CacheRoot == "" {
		return fmt.Errorf("external lint requires project, run, and cache paths")
	}
	if !filepath.IsAbs(request.ProjectRoot) || !filepath.IsAbs(request.RunDir) || !filepath.IsAbs(request.CacheRoot) {
		return fmt.Errorf("external lint paths must be absolute")
	}
	if !lintIdentifier.MatchString(request.ProjectID) || !lintIdentifier.MatchString(request.SessionID) || !lintIdentifier.MatchString(request.RunID) || !lintIdentifier.MatchString(request.TargetID) {
		return fmt.Errorf("external lint session, run, project, and target IDs must be safe identifiers")
	}
	if request.Provider == "" || request.Target.Mode == "" || request.Target.TargetPath == "" {
		return fmt.Errorf("external lint requires provider and resolved target identity")
	}
	return validateLintMatrix(request)
}

func containerName(request LintRequest) string {
	identity := strings.Join([]string{request.ProjectID, request.SessionID, request.RunID, request.TargetID}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "govard-audit-" + hex.EncodeToString(sum[:12]) + "-lint"
}

func imageRepository(image string) string {
	image = strings.SplitN(image, "@", 2)[0]
	lastSlash := strings.LastIndex(image, "/")
	if lastColon := strings.LastIndex(image, ":"); lastColon > lastSlash {
		return image[:lastColon]
	}
	return image
}

func imageWithDigest(image, digest string) string {
	return imageRepository(image) + "@" + digest
}

func isImmutableDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func readLintReport(path string) (LintReport, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return LintReport{}, fmt.Errorf("read external lint report: %w", err)
	}
	var report LintReport
	if err := json.Unmarshal(content, &report); err != nil {
		return LintReport{}, fmt.Errorf("decode external lint report: %w", err)
	}
	return report, nil
}
