package audit

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	auditmagento "govard/docker/audit"
)

// GovardGlintDigest pins the released official lint image (glint, formerly
// magelint) by its immutable content digest. It defaults to empty for dev
// builds; release automation injects the real value via
// "-X govard/internal/audit.GovardGlintDigest=<digest>" ldflags (and the
// legacy "-X govard/internal/audit.GovardMagelintDigest" is kept as alias).
// When empty, ToolchainManager never attempts to pull the official image.
var GovardGlintDigest string

// GovardMagelintDigest is the legacy alias for GovardGlintDigest (kept so
// "-X govard/internal/audit.GovardMagelintDigest" ldflags still work).
var GovardMagelintDigest string

func govardLintDigest() string {
	if v := strings.TrimSpace(GovardGlintDigest); v != "" {
		return v
	}
	return strings.TrimSpace(GovardMagelintDigest)
}

// officialGlintRepository is the Govard-owned official image repository
// (glint, formerly magelint).
const officialGlintRepository = "ghcr.io/ddtcorex/govard-magelint"

// officialMagelintRepository is the legacy alias for officialGlintRepository.
const officialMagelintRepository = officialGlintRepository

const materializedContextMarkerFilename = ".materialized"

// ResolvedToolchain is the outcome of ToolchainManager.Ensure: a ready image
// to run the Govard lint entrypoint in, and how it was obtained.
type ResolvedToolchain struct {
	Image         string
	ImageDigest   string
	ContextDigest string
	LocalBuild    bool
	Labels        map[string]string
}

// ToolchainManager resolves a ready-to-run Govard Magento lint image. It
// prefers the pinned official image when a release digest is known and its
// labels verify against the embedded build context, and otherwise falls back
// to building the embedded context locally. It never invokes an externally
// configured lint provider (see ExternalLintProvider) and never runs a
// container itself; it only manages the image.
type ToolchainManager struct {
	docker    DockerClient
	cacheRoot string
	contextFS fs.FS
}

// NewToolchainManager builds a manager rooted at the given Govard home
// directory (the same string engine.GovardHomeDir() resolves to; this
// package never imports internal/engine). The materialized build context
// cache lives under DefaultToolchainCacheRoot(govardHome).
func NewToolchainManager(docker DockerClient, govardHome string) *ToolchainManager {
	return &ToolchainManager{
		docker:    docker,
		cacheRoot: DefaultToolchainCacheRoot(govardHome),
		contextFS: auditmagento.ContextFS,
	}
}

// DefaultToolchainCacheRoot derives the toolchain cache root from an
// explicit Govard home directory, mirroring DefaultStoreRoot.
func DefaultToolchainCacheRoot(govardHome string) string {
	return filepath.Join(govardHome, "cache", "audit", "toolchains")
}

// ToolchainStatus reports which lint toolchain image is already usable on this
// host. It is produced by Status, which never pulls and never builds, so it
// describes the current machine rather than what a run would end up creating.
type ToolchainStatus struct {
	// Present is true when Toolchain names an image Ensure could reuse as-is.
	Present bool
	// Toolchain is the image Ensure would reuse right now, preferring the
	// pinned official image exactly like Ensure does. It stays the zero value
	// when nothing usable is present.
	Toolchain ResolvedToolchain
	// ContextDigest is this build's embedded lint context digest.
	ContextDigest string
	// OfficialImage is the pinned official reference, empty when this build has
	// no release digest injected or the injected one is malformed.
	OfficialImage string
	// OfficialUsable is true when the pinned official image is already local
	// and its labels verify against this build's embedded context.
	OfficialUsable bool
	// LocalBuildImage is the content-addressed embedded-build tag for this
	// build's context digest.
	LocalBuildImage string
	// LocalBuildPresent is true when that image already exists on this host.
	LocalBuildPresent bool
}

// Ensure resolves a ready-to-run image: the pinned official image when it is
// configured and verifies, otherwise a local build of the embedded context.
// Cancellation propagates immediately and never triggers the embedded build
// fallback, since a cancelled run should not pay for an unrequested build.
func (manager *ToolchainManager) Ensure(ctx context.Context) (ResolvedToolchain, error) {
	contextDigest, err := manager.begin(ctx, "ensure")
	if err != nil {
		return ResolvedToolchain{}, err
	}

	if raw := govardLintDigest(); raw != "" {
		imageRef, pinnedDigest, ok := officialImageReference(raw)
		if !ok {
			// A malformed release digest is a Task 9 release-pipeline bug,
			// not a transient runtime condition, but it must not brick every
			// user's local audit run for the life of a release: degrade
			// exactly like every other official-path failure below (pull
			// failure, inspect failure, wrong labels) and fall back to the
			// embedded build. Surface it as a diagnostic so a broken release
			// is still visible, just not fatal.
			slog.Warn("configured official audit lint image digest is malformed; falling back to the embedded build", "digest", raw)
		} else {
			resolved, usable, err := manager.ensureOfficialImage(ctx, imageRef, pinnedDigest, contextDigest)
			if err != nil {
				return ResolvedToolchain{}, err
			}
			if usable {
				return resolved, nil
			}
		}
	}

	return manager.ensureLocalBuild(ctx, contextDigest)
}

// begin performs the checks every toolchain operation shares: a usable manager,
// a real context, this build's embedded context digest, and an early
// cancellation stop before any Docker work is attempted.
func (manager *ToolchainManager) begin(ctx context.Context, operation string) (string, error) {
	if manager == nil || manager.docker == nil {
		return "", fmt.Errorf("audit lint toolchain manager is not configured")
	}
	if ctx == nil {
		return "", fmt.Errorf("audit lint toolchain %s context is nil", operation)
	}
	contextDigest, err := auditmagento.ContextDigest()
	if err != nil {
		return "", fmt.Errorf("compute embedded audit lint context digest: %w", err)
	}
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return "", fmt.Errorf("audit lint toolchain %s cancelled: %w", operation, cause)
	}
	return contextDigest, nil
}

// Pull resolves only the pinned official image and never builds anything. It
// exists so an operator can exercise or force the official path explicitly;
// Ensure remains the path that degrades to the embedded build, and a failure
// here therefore reports the official path as unusable instead of quietly
// building a local image the caller did not ask for. Operator guidance about
// what to run next belongs to the command layer, so no message here names a
// CLI command.
func (manager *ToolchainManager) Pull(ctx context.Context) (ResolvedToolchain, error) {
	contextDigest, err := manager.begin(ctx, "pull")
	if err != nil {
		return ResolvedToolchain{}, err
	}
	raw := govardLintDigest()
	if raw == "" {
		return ResolvedToolchain{}, fmt.Errorf("this build pins no official audit lint image digest, so there is nothing to pull")
	}
	imageRef, pinnedDigest, ok := officialImageReference(raw)
	if !ok {
		return ResolvedToolchain{}, fmt.Errorf("the official audit lint image digest configured for this build is not a valid sha256 digest")
	}
	resolved, usable, err := manager.ensureOfficialImage(ctx, imageRef, pinnedDigest, contextDigest)
	if err != nil {
		return ResolvedToolchain{}, err
	}
	if !usable {
		return ResolvedToolchain{}, fmt.Errorf("the pinned official audit lint image %s could not be pulled or does not match this build's embedded lint context", imageRef)
	}
	return resolved, nil
}

// Build builds only the embedded context and never pulls. An already-built
// image for the same context digest is reused as-is, since it is content
// addressed and rebuilding it could not produce anything different.
func (manager *ToolchainManager) Build(ctx context.Context) (ResolvedToolchain, error) {
	contextDigest, err := manager.begin(ctx, "build")
	if err != nil {
		return ResolvedToolchain{}, err
	}
	return manager.ensureLocalBuild(ctx, contextDigest)
}

// Status inspects what is already available without pulling, building, or
// writing anything. An image Docker cannot inspect is reported as absent rather
// than as an error, because "not present yet" is the normal answer here.
func (manager *ToolchainManager) Status(ctx context.Context) (ToolchainStatus, error) {
	contextDigest, err := manager.begin(ctx, "status")
	if err != nil {
		return ToolchainStatus{}, err
	}
	localImage, err := localBuildImage(contextDigest)
	if err != nil {
		return ToolchainStatus{}, err
	}
	status := ToolchainStatus{ContextDigest: contextDigest, LocalBuildImage: localImage}

	if raw := govardLintDigest(); raw != "" {
		if imageRef, pinnedDigest, ok := officialImageReference(raw); ok {
			status.OfficialImage = imageRef
			if inspection, inspectErr := manager.docker.Inspect(ctx, imageRef); inspectErr == nil {
				if resolved, verified := officialToolchainFromInspection(imageRef, pinnedDigest, contextDigest, inspection); verified {
					status.OfficialUsable = true
					status.Present = true
					status.Toolchain = resolved
				}
			}
		}
	}

	if inspection, inspectErr := manager.docker.Inspect(ctx, localImage); inspectErr == nil {
		status.LocalBuildPresent = true
		if !status.Present {
			status.Present = true
			status.Toolchain = ResolvedToolchain{
				Image:         localImage,
				ImageDigest:   inspection.ID,
				ContextDigest: contextDigest,
				LocalBuild:    true,
				Labels:        copyStringMap(inspection.Labels),
			}
		}
	}

	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ToolchainStatus{}, fmt.Errorf("audit lint toolchain status cancelled: %w", cause)
	}
	return status, nil
}

// ensureOfficialImage resolves the pinned official image without ever
// falling back to a floating tag. A non-nil error means cancellation (or an
// equivalent hard stop) and callers must not attempt the embedded build
// fallback in that case. A false "usable" with a nil error means the
// official image could not be obtained or does not verify, and the embedded
// build fallback should be attempted instead.
func (manager *ToolchainManager) ensureOfficialImage(ctx context.Context, imageRef, pinnedDigest, contextDigest string) (ResolvedToolchain, bool, error) {
	inspection, inspectErr := manager.docker.Inspect(ctx, imageRef)
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ResolvedToolchain{}, false, fmt.Errorf("audit lint toolchain ensure cancelled while inspecting the official image: %w", cause)
	}
	if inspectErr == nil {
		// The image is already local and pinned by digest, so there is
		// nothing a pull could change: either the labels verify and it is
		// reused as-is, or they do not and no pull will fix that.
		resolved, ok := officialToolchainFromInspection(imageRef, pinnedDigest, contextDigest, inspection)
		return resolved, ok, nil
	}

	pullErr := manager.docker.Pull(ctx, imageRef)
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ResolvedToolchain{}, false, fmt.Errorf("audit lint toolchain ensure cancelled while pulling the official image: %w", cause)
	}
	if pullErr != nil {
		return ResolvedToolchain{}, false, nil
	}

	pulledInspection, pulledInspectErr := manager.docker.Inspect(ctx, imageRef)
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ResolvedToolchain{}, false, fmt.Errorf("audit lint toolchain ensure cancelled while inspecting the pulled official image: %w", cause)
	}
	if pulledInspectErr != nil {
		return ResolvedToolchain{}, false, nil
	}
	resolved, ok := officialToolchainFromInspection(imageRef, pinnedDigest, contextDigest, pulledInspection)
	return resolved, ok, nil
}

func officialToolchainFromInspection(imageRef, pinnedDigest, contextDigest string, inspection ImageInspection) (ResolvedToolchain, bool) {
	if !officialLabelsVerify(inspection.Labels, contextDigest) {
		return ResolvedToolchain{}, false
	}
	return ResolvedToolchain{
		Image:         imageRef,
		ImageDigest:   pinnedDigest,
		ContextDigest: contextDigest,
		LocalBuild:    false,
		Labels:        copyStringMap(inspection.Labels),
	}, true
}

// officialLabelsVerify checks the exact labels the official image must carry.
// The context digest, report schema, and PHP-version set must match this
// build's embedded context exactly. The OCI revision and Govard version
// labels only need to be present and non-empty: this layer has no
// independently known expected value for either.
func officialLabelsVerify(labels map[string]string, contextDigest string) bool {
	if strings.TrimSpace(labels["org.opencontainers.image.revision"]) == "" {
		return false
	}
	if strings.TrimSpace(labels["io.govard.version"]) == "" {
		return false
	}
	if labels["io.govard.audit.context-digest"] != contextDigest {
		return false
	}
	if labels["io.govard.audit.report-schema"] != strconv.Itoa(auditmagento.ReportSchemaVersion) {
		return false
	}
	if labels["io.govard.audit.php-versions"] != strings.Join(auditmagento.PHPVersions(), ",") {
		return false
	}
	return true
}

// officialImageReference pins the configured release digest into an
// immutable image reference. It accepts the digest with or without a
// "sha256:" prefix so it stays compatible regardless of the exact form
// Task 9's release automation ends up injecting. A false "ok" means the
// configured value is not a valid sha256 digest; callers must treat that as
// just another official-path failure (fall back to the embedded build), not
// a fatal error, since this value is baked in at build time and would
// otherwise brick the official-image path for the entire life of a release.
func officialImageReference(raw string) (imageRef, pinnedDigest string, ok bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "sha256:")
	pinnedDigest = "sha256:" + trimmed
	if !isImmutableDigest(pinnedDigest) {
		return "", "", false
	}
	return officialMagelintRepository + "@" + pinnedDigest, pinnedDigest, true
}

// ensureLocalBuild reuses the content-addressed local build image when it
// already exists, and otherwise materializes the embedded build context and
// builds it.
func (manager *ToolchainManager) ensureLocalBuild(ctx context.Context, contextDigest string) (ResolvedToolchain, error) {
	localImage, err := localBuildImage(contextDigest)
	if err != nil {
		return ResolvedToolchain{}, err
	}

	if inspection, err := manager.docker.Inspect(ctx, localImage); err == nil {
		return ResolvedToolchain{
			Image:         localImage,
			ImageDigest:   inspection.ID,
			ContextDigest: contextDigest,
			LocalBuild:    true,
			Labels:        copyStringMap(inspection.Labels),
		}, nil
	}
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ResolvedToolchain{}, fmt.Errorf("audit lint toolchain ensure cancelled while inspecting the local build image: %w", cause)
	}

	contextDir, err := manager.materializeContext(contextDigest)
	if err != nil {
		return ResolvedToolchain{}, err
	}
	buildLabels := map[string]string{
		"io.govard.audit.context-digest": contextDigest,
		"io.govard.audit.report-schema":  strconv.Itoa(auditmagento.ReportSchemaVersion),
		"io.govard.audit.php-versions":   strings.Join(auditmagento.PHPVersions(), ","),
	}
	if err := manager.docker.Build(ctx, contextDir, localImage, buildLabels); err != nil {
		return ResolvedToolchain{}, fmt.Errorf("build embedded audit lint image: %w", err)
	}
	if cause := externalLintCancellationCause(ctx); cause != nil {
		return ResolvedToolchain{}, fmt.Errorf("audit lint toolchain ensure cancelled while building the embedded audit lint image: %w", cause)
	}

	builtInspection, err := manager.docker.Inspect(ctx, localImage)
	if err != nil {
		return ResolvedToolchain{}, fmt.Errorf("inspect built audit lint image: %w", err)
	}
	return ResolvedToolchain{
		Image:         localImage,
		ImageDigest:   builtInspection.ID,
		ContextDigest: contextDigest,
		LocalBuild:    true,
		Labels:        copyStringMap(builtInspection.Labels),
	}, nil
}

// localBuildImage derives the content-addressed local build tag from the
// embedded context digest: the first 16 hex characters of its hex portion
// (glint, formerly magelint).
func localBuildImage(contextDigest string) (string, error) {
	trimmed := strings.TrimPrefix(contextDigest, "sha256:")
	if len(trimmed) < 16 {
		return "", fmt.Errorf("embedded audit lint context digest %q is too short to derive a build tag", contextDigest)
	}
	return "govard-local/glint:" + trimmed[:16], nil
}

// materializeContext writes the embedded build context out to
// "<cacheRoot>/context/<hex-digest>" so a real docker build has a real
// filesystem context to point at. It is defensive against any embedded path
// escaping that directory (there should never be one, since the context is
// Govard's own embedded FS, but the check costs nothing) and materializes
// atomically via a temporary staging directory plus rename, so a crash
// mid-write can never leave a partial directory that a later run mistakes
// for already materialized.
func (manager *ToolchainManager) materializeContext(contextDigest string) (string, error) {
	trimmed := strings.TrimPrefix(contextDigest, "sha256:")
	if trimmed == "" {
		return "", fmt.Errorf("embedded audit lint context digest is empty")
	}
	contextRoot := filepath.Join(manager.cacheRoot, "context")
	destination := filepath.Join(contextRoot, trimmed)
	marker := filepath.Join(destination, materializedContextMarkerFilename)
	if info, statErr := os.Stat(marker); statErr == nil && info.Mode().IsRegular() {
		return destination, nil
	}

	if err := os.MkdirAll(contextRoot, 0o755); err != nil {
		return "", fmt.Errorf("create audit lint context cache directory: %w", err)
	}
	staging, err := os.MkdirTemp(contextRoot, ".materializing-*")
	if err != nil {
		return "", fmt.Errorf("create audit lint context staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	paths, err := auditmagento.ContextPaths()
	if err != nil {
		return "", err
	}
	stagingRoot := filepath.Clean(staging) + string(filepath.Separator)
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
			return "", fmt.Errorf("embedded audit lint context path %q escapes its root", path)
		}
		target := filepath.Join(staging, cleaned)
		if !strings.HasPrefix(target, stagingRoot) {
			return "", fmt.Errorf("embedded audit lint context path %q escapes the materialized directory", path)
		}
		content, err := fs.ReadFile(manager.contextFS, path)
		if err != nil {
			return "", fmt.Errorf("read embedded audit lint context %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("create audit lint context directory for %q: %w", path, err)
		}
		if err := os.WriteFile(target, content, auditmagento.FileMode(path)); err != nil {
			return "", fmt.Errorf("write audit lint context file %q: %w", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(staging, materializedContextMarkerFilename), []byte(contextDigest), 0o644); err != nil {
		return "", fmt.Errorf("write audit lint context marker: %w", err)
	}

	if err := os.Rename(staging, destination); err != nil {
		if info, statErr := os.Stat(marker); statErr == nil && info.Mode().IsRegular() {
			// Another process materialized the same digest concurrently;
			// its content is identical by construction, so reuse it.
			return destination, nil
		}
		return "", fmt.Errorf("materialize audit lint context: %w", err)
	}
	committed = true
	return destination, nil
}

func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
