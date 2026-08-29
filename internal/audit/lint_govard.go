package audit

import (
	"bytes"
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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"
	auditmagento "govard/docker/audit-magento"
	"govard/internal/frameworks/types"
)

// GovardLintProvider is the provider name of the Govard-owned Magento lint
// backend. It is the value the image entrypoint reports back, so a report from
// any other provider can never be mistaken for Govard's own evidence.
const GovardLintProvider = "govard"

// Container paths the Govard lint image entrypoint speaks. Only the cache and
// output mounts are writable; the source and credential mounts are read only.
const (
	govardLintSourceMount = "/source"
	govardLintCacheMount  = "/cache"
	govardLintOutputMount = "/output"
	govardLintAuthMount   = "/auth/auth.json"
	govardLintSSHMount    = "/ssh-agent"
	govardLintReportMount = govardLintOutputMount + "/report.json"
)

const (
	govardLintReportFilename    = "report.json"
	govardLintLogFilename       = "govard-lint.log"
	govardLintQuarantineDirname = "quarantine"
	govardLintQuarantinePrefix  = "report-"
	govardLintQuarantineNibbles = 16
	govardLintMinJobs           = 1
	govardLintMaxJobs           = 32
	govardLintDigestNibbles     = 32
)

// Cache generation layout. A generation directory is keyed on the toolchain
// digest and is the writable /cache mount; the entrypoint owns "analyzer" and
// "composer" beneath it.
const (
	govardLintCacheInputsFilename  = ".govard-lint-inputs"
	govardLintAnalyzerCacheDirname = "analyzer"
	govardLintCacheGenerationsKept = 2
	govardLintCacheGenerationGrace = time.Hour
)

// Exit codes the image entrypoint documents. Only these five mean the runner
// completed and published a report; everything else (a usage error, a Docker
// failure) is an infrastructure failure with no report to trust.
const (
	govardLintExitPassed      = 0
	govardLintExitFindings    = 1
	govardLintExitInfraError  = 2
	govardLintExitUnsupported = 3
	govardLintExitUsage       = 64
	govardLintExitCancelled   = 143
)

// Report statuses and per-PHP outcomes shared with the report schema.
const (
	lintStatusPassed      = "passed"
	lintStatusFailed      = "failed"
	lintStatusUnsupported = "unsupported"
	lintStatusInfraError  = "infra_error"
	lintStatusCancelled   = "cancelled"
)

// Cache states the runner may report for one PHP version.
const (
	lintCacheCold     = "cold"
	lintCacheWarm     = "warm"
	lintCacheBypassed = "bypassed"
)

// Project inputs that must invalidate the reusable analyzer cache even when
// the image and the toolchain digest are unchanged.
var (
	govardLintManifestFiles = []string{"composer.json", "composer.lock"}
	govardLintAnalyzerFiles = []string{"phpcs.xml", "phpcs.xml.dist", "phpstan.neon", "phpstan.neon.dist", "phpstan.dist.neon"}
)

// GovardLintOptions configures the Govard-owned lint backend. The Composer
// auth file and the SSH agent socket are opt-in host resources: neither is
// ever mounted unless the caller supplies it, and the agent additionally
// requires AllowSSHAgent.
//
// UID and GID must be the real host identity. The lint image declares no USER,
// so a container without an explicit user runs as root, and both report writers
// publish the report with mode 0600 - a root-owned 0600 report is unreadable by
// the host user this process runs as, which surfaces much later as an
// undecodable report rather than as the permission problem it is. Because the
// zero value of an int is a valid UID (root), a caller that simply forgets to
// resolve the host identity would otherwise land on root silently, so a
// non-positive UID or GID is rejected unless AllowRootUser opts into it
// deliberately.
type GovardLintOptions struct {
	Toolchain     *ToolchainManager
	Docker        DockerClient
	AuthJSON      string
	SSHAgent      string
	AllowSSHAgent bool
	UID           int
	GID           int
	// AllowRootUser deliberately permits a container user Govard cannot read
	// reports back from: UID 0 (explicit root) or a negative UID (no --user at
	// all, which the image resolves to root as well). It exists so that choice
	// is always explicit, never the result of an unset field.
	AllowRootUser bool
}

// GovardLintBackend runs the Govard-owned Magento lint image. It is the
// default backend and is unrelated to ExternalLintProvider, which only ever
// runs an explicitly configured third-party command.
type GovardLintBackend struct {
	toolchain     *ToolchainManager
	docker        DockerClient
	authJSON      string
	sshAgent      string
	allowSSHAgent bool
	uid           int
	gid           int
}

// DefaultLintCacheRoot derives the reusable lint cache root from an explicit
// Govard home directory (the same string engine.GovardHomeDir() resolves to;
// this package never imports internal/engine), mirroring DefaultStoreRoot.
// Per-target and per-fingerprint directories are created beneath it.
func DefaultLintCacheRoot(govardHome string) string {
	return filepath.Join(govardHome, "cache", "audit", "lint")
}

func NewGovardLintBackend(options GovardLintOptions) (*GovardLintBackend, error) {
	if options.Toolchain == nil {
		return nil, fmt.Errorf("govard lint backend requires a toolchain manager")
	}
	if options.AuthJSON != "" && !filepath.IsAbs(options.AuthJSON) {
		return nil, fmt.Errorf("govard lint Composer auth path must be absolute")
	}
	if options.AllowSSHAgent && options.SSHAgent != "" && !filepath.IsAbs(options.SSHAgent) {
		return nil, fmt.Errorf("govard lint SSH agent socket path must be absolute")
	}
	if err := validateGovardLintContainerUser(options); err != nil {
		return nil, err
	}
	docker := options.Docker
	if docker == nil {
		docker = NewExecDockerClient(nil)
	}
	return &GovardLintBackend{
		toolchain:     options.Toolchain,
		docker:        docker,
		authJSON:      options.AuthJSON,
		sshAgent:      options.SSHAgent,
		allowSSHAgent: options.AllowSSHAgent,
		uid:           options.UID,
		gid:           options.GID,
	}, nil
}

// validateGovardLintContainerUser requires a real host identity. Running the
// lint container as root leaves a mode-0600 report this process cannot read, so
// that outcome must be opted into rather than reached by leaving UID unset.
func validateGovardLintContainerUser(options GovardLintOptions) error {
	if options.AllowRootUser {
		return nil
	}
	if options.UID <= 0 || options.GID <= 0 {
		return fmt.Errorf("govard lint backend requires the host user and group IDs (got uid %d, gid %d); the image declares no user, so the container would run as root and publish a mode-0600 report this process cannot read - set AllowRootUser to accept that deliberately", options.UID, options.GID)
	}
	return nil
}

func isMissingCodingStandardError(msg string) bool {
	return strings.Contains(msg, "was not installed") || strings.Contains(msg, "ERROR: the")
}

func (backend *GovardLintBackend) Name() string {
	return GovardLintProvider
}

// Run resolves the toolchain image, executes the lint runner once for the
// whole requested PHP matrix, and accepts its report only when every identity
// field matches this run. Cancellation stops the container and then removes
// it, and always reports back as cancelled.
func (backend *GovardLintBackend) Run(ctx context.Context, request LintRequest) (LintReport, error) {
	if backend == nil || backend.toolchain == nil || backend.docker == nil {
		return LintReport{}, fmt.Errorf("govard lint backend is not configured")
	}
	if ctx == nil {
		return LintReport{}, fmt.Errorf("govard lint context is nil")
	}
	if request.Provider != GovardLintProvider {
		return LintReport{}, fmt.Errorf("govard lint request provider %q does not match provider %q", request.Provider, GovardLintProvider)
	}
	if err := validateLintRequest(request); err != nil {
		return LintReport{}, err
	}
	plan, err := govardLintTargetPlan(request)
	if err != nil {
		return LintReport{}, err
	}
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return cancelledGovardLintResult(cause, nil)
	}
	if err := os.MkdirAll(request.RunDir, 0o700); err != nil {
		return LintReport{}, fmt.Errorf("create govard lint run directory: %w", err)
	}
	// A report left behind by an earlier run must never be mistaken for this
	// run's result, so it is removed before the container can write anything.
	reportPath := filepath.Join(request.RunDir, govardLintReportFilename)
	if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return LintReport{}, fmt.Errorf("remove prior govard lint report: %w", err)
	}

	resolved, err := backend.toolchain.Ensure(ctx)
	if err != nil {
		return govardLintToolchainFailure(ctx, err)
	}
	if strings.TrimSpace(resolved.Image) == "" || strings.TrimSpace(resolved.ImageDigest) == "" {
		return LintReport{}, fmt.Errorf("resolved govard lint toolchain has no exact image identity")
	}
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return cancelledGovardLintResult(cause, nil)
	}
	logFile, err := os.OpenFile(filepath.Join(request.RunDir, govardLintLogFilename), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return LintReport{}, fmt.Errorf("open govard lint log: %w", err)
	}
	defer func() {
		if cerr := logFile.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "govard: close govard lint log %s: %v\n", logFile.Name(), cerr)
		}
	}()

	originalStandard := strings.TrimSpace(request.Profile.CodingStandard)
	currentRequest := request
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		toolchainDigest := govardLintToolchainDigest(resolved, currentRequest)
		cacheDir, err := backend.cacheDirectory(currentRequest, plan, resolved, toolchainDigest)
		if err != nil {
			return LintReport{}, err
		}
		container := backend.containerRequest(currentRequest, plan, resolved, toolchainDigest, cacheDir)
		// Only container-safe values are logged: no credential path, no secret,
		// and no environment dump.
		_, _ = fmt.Fprintf(logFile, "govard lint %s image %s args %s\n", container.Name, container.Image, strings.Join(container.Args, " "))
		if cause := externalLintCancellationCause(ctx); cause != nil {
			return cancelledGovardLintResult(cause, nil)
		}
		var outputBuf bytes.Buffer
		output := io.Writer(io.MultiWriter(logFile, &outputBuf))
		if request.StreamWriter != nil {
			output = io.MultiWriter(logFile, &outputBuf, request.StreamWriter)
		}
		runErr := backend.runContainer(ctx, container, output)
		if cause := externalLintCancellationCause(ctx); cause != nil {
			return LintReport{Status: lintStatusCancelled}, govardLintCancelledError(cause, backend.cleanupContainer(container.Name, logFile))
		}
		combinedMsg := ""
		if runErr != nil {
			combinedMsg = runErr.Error()
		}
		combinedMsg += outputBuf.String()
		// Also include log tail for cases where error is only in output
		if isMissingCodingStandardError(combinedMsg) && attempt == 0 && originalStandard != "" && originalStandard != "PSR12" {
			pterm.Warning.Printf("CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
			_, _ = fmt.Fprintf(logFile, "govard lint warning: CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
			_ = backend.removeCompletedContainer(container.Name, logFile)
			if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return LintReport{}, fmt.Errorf("remove prior govard lint report: %w", err)
			}
			currentRequest.Profile.CodingStandard = "PSR12"
			lastErr = runErr
			continue
		}
		exitCode, completed := govardLintExitStatus(runErr)
		if !completed {
			primary := govardLintInfrastructureExitError(exitCode, runErr)
			// Also consider fallback on infra error that contains missing standard message
			if isMissingCodingStandardError(combinedMsg+primary.Error()) && attempt == 0 && originalStandard != "" && originalStandard != "PSR12" {
				pterm.Warning.Printf("CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
				_, _ = fmt.Fprintf(logFile, "govard lint warning: CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
				_ = backend.removeCompletedContainer(container.Name, logFile)
				if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return LintReport{}, fmt.Errorf("remove prior govard lint report: %w", err)
				}
				currentRequest.Profile.CodingStandard = "PSR12"
				lastErr = primary
				continue
			}
			return LintReport{}, withGovardLintCleanupError(primary, backend.removeCompletedContainer(container.Name, logFile))
		}
		cleanupErr := backend.removeCompletedContainer(container.Name, logFile)
		report, err := backend.acceptReport(currentRequest, resolved, toolchainDigest, exitCode, reportPath, logFile)
		if err != nil {
			if isMissingCodingStandardError(combinedMsg+err.Error()) && attempt == 0 && originalStandard != "" && originalStandard != "PSR12" {
				pterm.Warning.Printf("CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
				_, _ = fmt.Fprintf(logFile, "govard lint warning: CodingStandard %q not found in lint image, falling back to PSR12\n", originalStandard)
				currentRequest.Profile.CodingStandard = "PSR12"
				lastErr = err
				_ = cleanupErr
				continue
			}
			return LintReport{}, withGovardLintCleanupError(err, cleanupErr)
		}
		return report, nil
	}
	if lastErr != nil {
		return LintReport{}, lastErr
	}
	return LintReport{}, fmt.Errorf("govard lint failed after fallback")
}

// acceptReport validates every identity, cache, and status field before a
// container-produced report becomes audit evidence. Anything unusable is
// quarantined instead of being read again by a later step.
func (backend *GovardLintBackend) acceptReport(request LintRequest, resolved ResolvedToolchain, toolchainDigest string, exitCode int, reportPath string, log io.Writer) (LintReport, error) {
	report, err := readLintReport(reportPath)
	if err != nil {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, "the report could not be decoded", err, log)
	}
	if err := ValidateLintReport(request, report); err != nil {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, "the report identity does not match this run", err, log)
	}
	if report.ImageDigest != resolved.ImageDigest {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, "the report identity does not match this run", errors.New("report image digest does not match the resolved toolchain image"), log)
	}
	if report.ToolchainDigest != toolchainDigest {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, "the report identity does not match this run", errors.New("report toolchain digest does not match the resolved toolchain"), log)
	}
	if err := govardLintCacheEvidence(request, report); err != nil {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, "the report cache evidence is invalid", err, log)
	}
	if want := govardLintStatusForExit(exitCode); report.Status != want {
		return LintReport{}, quarantineGovardLintReport(request, reportPath, fmt.Sprintf("the report status does not match runner exit code %d", exitCode), fmt.Errorf("report status %q is not the expected %q", report.Status, want), log)
	}
	return report, nil
}

func (backend *GovardLintBackend) containerRequest(request LintRequest, plan govardLintTarget, resolved ResolvedToolchain, toolchainDigest, cacheDir string) ContainerRunRequest {
	environment := map[string]string{
		"GOVARD_LINT_PROVIDER":         GovardLintProvider,
		"GOVARD_LINT_SESSION_ID":       request.SessionID,
		"GOVARD_LINT_RUN_ID":           request.RunID,
		"GOVARD_LINT_PROJECT_ID":       request.ProjectID,
		"GOVARD_LINT_TARGET_ID":        request.TargetID,
		"GOVARD_LINT_TARGET_MODE":      string(plan.mode),
		"GOVARD_LINT_TARGET_PATH":      request.Target.TargetPath,
		"GOVARD_LINT_IMAGE_DIGEST":     resolved.ImageDigest,
		"GOVARD_LINT_TOOLCHAIN_DIGEST": toolchainDigest,
		"GOVARD_LINT_MATRIX_COMPLETE":  strconv.FormatBool(request.MatrixComplete),
	}
	if standard := strings.TrimSpace(request.Profile.CodingStandard); standard != "" {
		environment["GOVARD_LINT_CODING_STANDARD"] = standard
	}
	if request.Profile.PHPStanLevel > 0 {
		environment["GOVARD_LINT_PHPSTAN_LEVEL"] = strconv.Itoa(request.Profile.PHPStanLevel)
	}
	container := ContainerRunRequest{
		Name:  containerName(request),
		Image: resolved.Image,
		User:  govardLintContainerUser(backend.uid, backend.gid),
		Args:  govardLintArguments(request, plan),
		Mounts: []ContainerMount{
			{Source: plan.source, Target: govardLintSourceMount, ReadOnly: true},
			{Source: cacheDir, Target: govardLintCacheMount},
			{Source: request.RunDir, Target: govardLintOutputMount},
		},
		Environment: environment,
		Labels: map[string]string{
			"io.govard.audit.provider": GovardLintProvider,
			"io.govard.audit.session":  request.SessionID,
			"io.govard.audit.run":      request.RunID,
			"io.govard.audit.project":  request.ProjectID,
			"io.govard.audit.target":   request.TargetID,
		},
	}
	// Credentials are mounted read only at the path the entrypoint reads by
	// default; the runner links them into a private Composer home and never
	// copies, logs, or reports them.
	if backend.authJSON != "" {
		container.Mounts = append(container.Mounts, ContainerMount{Source: backend.authJSON, Target: govardLintAuthMount, ReadOnly: true})
	}
	// SSH forwarding is a plain socket bind mount plus SSH_AUTH_SOCK, and is
	// strictly opt-in: the boolean gates it even when a socket is configured.
	if backend.allowSSHAgent && backend.sshAgent != "" {
		container.Mounts = append(container.Mounts, ContainerMount{Source: backend.sshAgent, Target: govardLintSSHMount})
		container.Environment["SSH_AUTH_SOCK"] = govardLintSSHMount
	}
	return container
}

func govardLintArguments(request LintRequest, plan govardLintTarget) []string {
	args := []string{"--target-mode", string(plan.mode)}
	if plan.relative != "" {
		args = append(args, "--target-relative", plan.relative)
	}
	args = append(args, "--php", strings.Join(request.PHPVersions(), ","))
	if linters := govardLintLinters(request.Profile.Linters); linters != "" {
		args = append(args, "--linter", linters)
	}
	args = append(args, "--jobs", strconv.Itoa(boundedGovardLintJobs(request.Jobs)))
	args = append(args, "--report", govardLintReportMount)
	if request.BypassResultCache {
		args = append(args, "--no-result-cache")
	}
	return args
}

func govardLintLinters(linters []string) string {
	selected := make([]string, 0, len(linters))
	for _, linter := range linters {
		if trimmed := strings.TrimSpace(linter); trimmed != "" {
			selected = append(selected, trimmed)
		}
	}
	return strings.Join(selected, ",")
}

// boundedGovardLintJobs keeps the analyzer worker count inside the range the
// runner accepts, so a caller-side default can never turn into a usage error.
func boundedGovardLintJobs(jobs int) int {
	if jobs < govardLintMinJobs {
		return govardLintMinJobs
	}
	if jobs > govardLintMaxJobs {
		return govardLintMaxJobs
	}
	return jobs
}

// govardLintContainerUser renders the host identity the container must run as.
// A negative UID means the caller has no identity to impose, so Docker keeps
// the image's own user rather than having one forced on it.
func govardLintContainerUser(uid, gid int) string {
	if uid < 0 {
		return ""
	}
	if gid < 0 {
		return strconv.Itoa(uid)
	}
	return strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
}

type govardLintTarget struct {
	source   string
	relative string
	mode     types.AuditTargetMode
}

// govardLintTargetPlan derives the read-only source mount and the relative
// path inside it. A standalone module mounts only itself, so nothing outside
// the module can influence its analysis.
func govardLintTargetPlan(request LintRequest) (govardLintTarget, error) {
	target := request.Target
	switch target.Mode {
	case types.AuditTargetStandalone:
		if !filepath.IsAbs(target.TargetPath) {
			return govardLintTarget{}, fmt.Errorf("govard lint standalone target path must be absolute")
		}
		return govardLintTarget{source: target.TargetPath, mode: target.Mode}, nil
	case types.AuditTargetProject, types.AuditTargetModule:
	default:
		return govardLintTarget{}, fmt.Errorf("govard lint cannot run target mode %q", target.Mode)
	}
	// Project and module targets analyze the single active project PHP
	// version. The runner rejects anything else as a usage error with no
	// report, so it is caught here with an actionable message instead.
	if len(request.SelectedPHPVersions) != 1 {
		return govardLintTarget{}, fmt.Errorf("govard lint %s targets analyze exactly one active PHP version, not %d", target.Mode, len(request.SelectedPHPVersions))
	}
	root := target.ProjectRoot
	if root == "" {
		root = request.ProjectRoot
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(target.TargetPath) {
		return govardLintTarget{}, fmt.Errorf("govard lint project and target paths must be absolute")
	}
	relative, err := filepath.Rel(root, target.TargetPath)
	if err != nil {
		return govardLintTarget{}, fmt.Errorf("resolve govard lint target inside its project: %w", err)
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == "." {
		relative = ""
	}
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return govardLintTarget{}, fmt.Errorf("govard lint target resolves outside its project root")
	}
	if target.Mode == types.AuditTargetProject && relative != "" {
		return govardLintTarget{}, fmt.Errorf("govard lint project target is not its own project root")
	}
	if target.Mode == types.AuditTargetModule && relative == "" {
		return govardLintTarget{}, fmt.Errorf("govard lint module target is its project root")
	}
	return govardLintTarget{source: root, relative: relative, mode: target.Mode}, nil
}

// govardLintToolchainDigest is the exact toolchain identity the runner must
// echo back in its report. It covers the image and the lint policy only, never
// project inputs: those belong to the reusable cache fingerprint instead.
func govardLintToolchainDigest(resolved ResolvedToolchain, request LintRequest) string {
	return LintToolchainDigest(LintToolchain{
		Provider:         GovardLintProvider,
		Image:            resolved.ImageDigest,
		Command:          []string{auditmagento.EntrypointPath},
		PHPVersions:      request.PHPVersions(),
		Linters:          request.Profile.Linters,
		CodingStandard:   request.Profile.CodingStandard,
		PHPStanLevel:     request.Profile.PHPStanLevel,
		PHPStanExtension: request.Profile.PHPStanExtension,
	})
}

// govardLintCacheFingerprint is the input set of the reusable analyzer cache.
// It is deliberately broader than the toolchain digest: a changed
// composer.lock or analyzer ruleset must invalidate cached analyzer state even
// when the image is identical. Credential contents and absolute paths are
// excluded so a cache directory name never carries either.
type govardLintCacheFingerprint struct {
	Provider         string   `json:"provider"`
	ImageDigest      string   `json:"image_digest"`
	ContextDigest    string   `json:"context_digest"`
	ReportSchema     int      `json:"report_schema"`
	Runner           string   `json:"runner"`
	PHPVersions      []string `json:"php_versions"`
	TargetMode       string   `json:"target_mode"`
	TargetRelative   string   `json:"target_relative"`
	Scope            string   `json:"scope"`
	Linters          []string `json:"linters"`
	CodingStandard   string   `json:"coding_standard"`
	PHPStanLevel     int      `json:"phpstan_level"`
	PHPStanExtension string   `json:"phpstan_extension"`
	Manifests        []string `json:"manifests"`
	Analyzers        []string `json:"analyzers"`
	ComposerAuth     bool     `json:"composer_auth"`
	SSHAgent         bool     `json:"ssh_agent"`
}

// cacheDirectory resolves the writable cache mount as
// "<cache root>/<target id>/<toolchain generation>". Both path elements are
// digests, so a cache directory name never exposes a literal source path.
//
// The generation is keyed on the toolchain digest, not on the broader project
// fingerprint, because the runner keeps its Composer download cache inside this
// same mount ("composer/"): keying the directory on composer.lock would create a
// fresh subtree and force a full cold re-download of the whole dependency tree
// on every lock change, while orphaning the previous subtree forever. The
// broader fingerprint is recorded inside the directory instead, and only the
// analyzer state ("analyzer/") is discarded when it changes.
func (backend *GovardLintBackend) cacheDirectory(request LintRequest, plan govardLintTarget, resolved ResolvedToolchain, toolchainDigest string) (string, error) {
	fingerprint, err := backend.cacheFingerprint(request, plan, resolved)
	if err != nil {
		return "", err
	}
	namespace := filepath.Join(request.CacheRoot, request.TargetID)
	generation := govardLintCacheGeneration(toolchainDigest)
	directory := filepath.Join(namespace, generation)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create govard lint cache directory: %w", err)
	}
	if err := govardLintSyncCacheInputs(directory, fingerprint); err != nil {
		return "", err
	}
	govardLintPruneCacheGenerations(namespace, generation, time.Now())
	return directory, nil
}

// govardLintCacheGeneration names the cache generation after the toolchain
// digest, which only changes with the image, runner, PHP matrix, or analyzer
// policy - not with the audited project's own files.
func govardLintCacheGeneration(toolchainDigest string) string {
	trimmed := strings.TrimPrefix(toolchainDigest, "sha256:")
	if len(trimmed) >= govardLintDigestNibbles {
		return trimmed[:govardLintDigestNibbles]
	}
	sum := sha256.Sum256([]byte(toolchainDigest))
	return hex.EncodeToString(sum[:])[:govardLintDigestNibbles]
}

// govardLintSyncCacheInputs records the broader project-input fingerprint in the
// cache generation. When it changed since the last run only the analyzer state
// is discarded, so a changed composer.lock or ruleset still invalidates cached
// analysis while the Composer download cache - the expensive part, and the whole
// reason a Composer cache exists - stays warm. The marker is rewritten on every
// run so its timestamp records the generation's last use.
func govardLintSyncCacheInputs(directory, fingerprint string) error {
	marker := filepath.Join(directory, govardLintCacheInputsFilename)
	previous, err := os.ReadFile(marker)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read govard lint cache inputs: %w", err)
	}
	if strings.TrimSpace(string(previous)) != fingerprint {
		if err := os.RemoveAll(filepath.Join(directory, govardLintAnalyzerCacheDirname)); err != nil {
			return fmt.Errorf("discard stale govard lint analyzer state: %w", err)
		}
	}
	if err := os.WriteFile(marker, []byte(fingerprint+"\n"), 0o600); err != nil {
		return fmt.Errorf("record govard lint cache inputs: %w", err)
	}
	return nil
}

// govardLintPruneCacheGenerations bounds the per-target cache namespace. Nothing
// else prunes reusable lint caches - Runner.CleanupOlderThan deliberately only
// removes persisted sessions - so an image upgrade or a changed PHP matrix would
// otherwise orphan its predecessor forever. The current generation and the most
// recently used ones are kept, and anything touched recently is left alone so a
// concurrent run never loses the cache underneath it. Pruning is best effort: a
// generation that cannot be removed is simply left in place.
func govardLintPruneCacheGenerations(namespace, current string, now time.Time) {
	entries, err := os.ReadDir(namespace)
	if err != nil {
		return
	}
	type generation struct {
		name string
		used time.Time
	}
	candidates := make([]generation, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current {
			continue
		}
		used := govardLintCacheGenerationUsed(namespace, entry.Name())
		if !used.IsZero() && now.Sub(used) < govardLintCacheGenerationGrace {
			continue
		}
		candidates = append(candidates, generation{name: entry.Name(), used: used})
	}
	if len(candidates) <= govardLintCacheGenerationsKept {
		return
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].used.After(candidates[right].used)
	})
	for _, stale := range candidates[govardLintCacheGenerationsKept:] {
		_ = os.RemoveAll(filepath.Join(namespace, stale.name))
	}
}

func govardLintCacheGenerationUsed(namespace, name string) time.Time {
	if info, err := os.Stat(filepath.Join(namespace, name, govardLintCacheInputsFilename)); err == nil {
		return info.ModTime()
	}
	if info, err := os.Stat(filepath.Join(namespace, name)); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

func (backend *GovardLintBackend) cacheFingerprint(request LintRequest, plan govardLintTarget, resolved ResolvedToolchain) (string, error) {
	scopes := govardLintFingerprintScopes(plan)
	manifests, err := govardLintFileFingerprints(scopes, govardLintManifestFiles)
	if err != nil {
		return "", err
	}
	analyzers, err := govardLintFileFingerprints(scopes, govardLintAnalyzerFiles)
	if err != nil {
		return "", err
	}
	fingerprint := govardLintCacheFingerprint{
		Provider:         GovardLintProvider,
		ImageDigest:      resolved.ImageDigest,
		ContextDigest:    resolved.ContextDigest,
		ReportSchema:     auditmagento.ReportSchemaVersion,
		Runner:           auditmagento.EntrypointPath,
		PHPVersions:      sortedStrings(request.PHPVersions()),
		TargetMode:       string(plan.mode),
		TargetRelative:   plan.relative,
		Scope:            string(request.Scope),
		Linters:          sortedStrings(request.Profile.Linters),
		CodingStandard:   request.Profile.CodingStandard,
		PHPStanLevel:     request.Profile.PHPStanLevel,
		PHPStanExtension: request.Profile.PHPStanExtension,
		Manifests:        manifests,
		Analyzers:        analyzers,
		// Only whether a private registry or agent could have been reachable
		// is recorded, never which one or with what credential.
		ComposerAuth: backend.authJSON != "",
		SSHAgent:     backend.allowSSHAgent && backend.sshAgent != "",
	}
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", fmt.Errorf("marshal govard lint cache fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type govardLintFingerprintScope struct {
	label     string
	directory string
}

func govardLintFingerprintScopes(plan govardLintTarget) []govardLintFingerprintScope {
	scopes := []govardLintFingerprintScope{{label: "root", directory: plan.source}}
	if plan.relative != "" {
		scopes = append(scopes, govardLintFingerprintScope{label: "target", directory: filepath.Join(plan.source, filepath.FromSlash(plan.relative))})
	}
	return scopes
}

// govardLintFileFingerprints records only a scope label, a file name, and a
// content digest, so no absolute host path enters the fingerprint.
func govardLintFileFingerprints(scopes []govardLintFingerprintScope, names []string) ([]string, error) {
	entries := make([]string, 0, len(scopes)*len(names))
	for _, scope := range scopes {
		for _, name := range names {
			digest, err := govardLintFileDigest(filepath.Join(scope.directory, name))
			if err != nil {
				return nil, err
			}
			entries = append(entries, scope.label+"/"+name+"="+digest)
		}
	}
	return entries, nil
}

func govardLintFileDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "absent", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("fingerprint govard lint cache input %q: %w", filepath.Base(path), err)
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func sortedStrings(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}

// govardLintCacheEvidence checks the cache state the runner reported per PHP
// version. A warm cache is a contradiction when the run explicitly bypassed
// reusable state, and an unknown state means the report cannot be trusted.
func govardLintCacheEvidence(request LintRequest, report LintReport) error {
	for _, result := range report.PHPResults {
		switch result.Cache.State {
		case lintCacheCold, lintCacheWarm, lintCacheBypassed:
		default:
			return fmt.Errorf("PHP %s reports unknown cache state %q", result.PHPVersion, result.Cache.State)
		}
		if request.BypassResultCache && result.Cache.State == lintCacheWarm {
			return fmt.Errorf("PHP %s reused a warm cache while the result cache was bypassed", result.PHPVersion)
		}
	}
	return nil
}

func govardLintStatusForExit(exitCode int) string {
	switch exitCode {
	case govardLintExitFindings:
		return lintStatusFailed
	case govardLintExitInfraError:
		return lintStatusInfraError
	case govardLintExitUnsupported:
		return lintStatusUnsupported
	case govardLintExitCancelled:
		return lintStatusCancelled
	default:
		return lintStatusPassed
	}
}

// govardLintExitStatus reports the runner exit code and whether that code
// means a report was published. A usage error (64) means this backend built a
// malformed invocation, so it is a programmer error with nothing to read.
func govardLintExitStatus(runErr error) (int, bool) {
	if runErr == nil {
		return govardLintExitPassed, true
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		return -1, false
	}
	code := exitErr.ExitCode()
	switch code {
	case govardLintExitPassed, govardLintExitFindings, govardLintExitInfraError, govardLintExitUnsupported, govardLintExitCancelled:
		return code, true
	}
	return code, false
}

func govardLintInfrastructureExitError(exitCode int, runErr error) error {
	if exitCode == govardLintExitUsage {
		return fmt.Errorf("govard lint runner rejected its invocation with a usage error (exit %d)", govardLintExitUsage)
	}
	if exitCode < 0 {
		return fmt.Errorf("govard lint container could not be run: %w", runErr)
	}
	return fmt.Errorf("govard lint container failed with infrastructure exit code %d", exitCode)
}

// quarantineGovardLintReport moves an unusable report out of the run directory
// so no later step can read it as evidence. Only the reason category, the
// quarantined path, and the content digest are surfaced; the detailed cause
// stays in the private run log, and the raw content is never reproduced.
func quarantineGovardLintReport(request LintRequest, reportPath, reason string, cause error, log io.Writer) error {
	content, err := os.ReadFile(reportPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			govardLintLogDiagnostic(log, reason, cause)
			return fmt.Errorf("govard lint published no report for this run")
		}
		govardLintLogDiagnostic(log, reason, cause)
		return fmt.Errorf("govard lint report could not be quarantined: %s", reason)
	}
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	// The quarantine directory needs owner search permission to hold anything,
	// so the directory is private (0700) while the report keeps mode 0600.
	directory := filepath.Join(request.RunDir, govardLintQuarantineDirname)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		govardLintLogDiagnostic(log, reason, cause)
		return fmt.Errorf("create govard lint quarantine directory: %w", err)
	}
	destination := filepath.Join(directory, govardLintQuarantinePrefix+hex.EncodeToString(sum[:])[:govardLintQuarantineNibbles]+".json")
	if err := os.Rename(reportPath, destination); err != nil {
		govardLintLogDiagnostic(log, reason, cause)
		return fmt.Errorf("quarantine govard lint report: %w", err)
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		govardLintLogDiagnostic(log, reason, cause)
		return fmt.Errorf("restrict quarantined govard lint report: %w", err)
	}
	govardLintLogDiagnostic(log, reason, cause)
	return fmt.Errorf("govard lint report was moved to quarantine %s (%s): %s", destination, digest, reason)
}

func govardLintLogDiagnostic(log io.Writer, reason string, cause error) {
	if log == nil {
		return
	}
	_, _ = fmt.Fprintf(log, "govard lint rejected its report: %s: %v\n", reason, cause)
}

func (backend *GovardLintBackend) runContainer(ctx context.Context, request ContainerRunRequest, output io.Writer) error {
	done := make(chan error, 1)
	go func() { done <- backend.docker.Run(ctx, request, output) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return externalLintCancellationCause(ctx)
	}
}

// cleanupContainer stops the container and then removes it, reusing the same
// bounded stop-then-remove pattern as the external provider so a hung Docker
// daemon can never block cancellation.
func (backend *GovardLintBackend) cleanupContainer(name string, output io.Writer) error {
	stopErr := labelExternalLintCleanupError("stop govard lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return backend.docker.Stop(ctx, name, externalLintStopTimeout)
	}))
	removeErr := labelExternalLintCleanupError("remove govard lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return backend.docker.Remove(ctx, name)
	}))
	err := errors.Join(stopErr, removeErr)
	if err != nil && output != nil {
		_, _ = fmt.Fprintf(output, "govard lint cleanup failed: %v\n", err)
	}
	return err
}

func (backend *GovardLintBackend) removeCompletedContainer(name string, output io.Writer) error {
	err := labelExternalLintCleanupError("remove completed govard lint container", runBoundedDockerCleanup(func(ctx context.Context) error {
		return backend.docker.Remove(ctx, name)
	}))
	if err != nil && output != nil {
		_, _ = fmt.Fprintf(output, "remove completed govard lint container failed: %v\n", err)
	}
	return err
}

// govardLintToolchainFailure keeps a cancelled toolchain resolution reported
// as cancelled rather than as a generic image failure.
func govardLintToolchainFailure(ctx context.Context, err error) (LintReport, error) {
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return LintReport{Status: lintStatusCancelled}, fmt.Errorf("govard lint cancelled while resolving its toolchain: %w", errors.Join(cause, err))
	}
	return LintReport{}, fmt.Errorf("resolve govard lint toolchain: %w", err)
}

func cancelledGovardLintResult(cause, operationErr error) (LintReport, error) {
	if operationErr != nil {
		return LintReport{Status: lintStatusCancelled}, fmt.Errorf("govard lint cancelled before container execution: %w", errors.Join(cause, operationErr))
	}
	return LintReport{Status: lintStatusCancelled}, fmt.Errorf("govard lint cancelled before container execution: %w", cause)
}

func govardLintCancelledError(cause, cleanupErr error) error {
	if cleanupErr != nil {
		return fmt.Errorf("govard lint cancelled; cleanup: %w", errors.Join(cause, cleanupErr))
	}
	return fmt.Errorf("govard lint cancelled: %w", cause)
}

func withGovardLintCleanupError(primary, cleanupErr error) error {
	if cleanupErr == nil {
		return primary
	}
	return fmt.Errorf("govard lint execution and cleanup failed: %w", errors.Join(primary, cleanupErr))
}
