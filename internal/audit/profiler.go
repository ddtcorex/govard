package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"govard/internal/frameworks/types"
)

const (
	profilerLeaseResource         = "diagnostics"
	profilerArtifactRelativePath  = "profiler/profile.csv"
	defaultProfilerCleanupTimeout = 10 * time.Second
)

// ProfilerRequest identifies one runtime profiler capture. The runner creates
// this value from an immutable audit run; the injected runtime owns only the
// temporary diagnostic configuration and capture transport.
type ProfilerRequest struct {
	ProjectRoot string
	ProjectID   string
	SessionID   string
	RunID       string
	URL         string
	Target      types.AuditTarget
}

// ProfilerRuntime performs the environment-specific profiler lifecycle. Its
// implementation is deliberately injected: the generic audit runner never
// mutates web-server configuration or assumes a particular framework runtime.
type ProfilerRuntime interface {
	Activate(context.Context, ProfilerRequest) error
	Capture(context.Context, ProfilerRequest) error
	Collect(context.Context, ProfilerRequest) ([]byte, error)
	Restore(context.Context, ProfilerRequest) error
}

// ValidateProfilerURL rejects anything that cannot be fetched directly by the
// bounded HTTP client. The URL is never passed through a shell.
func ValidateProfilerURL(raw string) error {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return errors.New("audit profiler requires --url with an absolute http or https URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("audit profiler requires --url with an absolute http or https URL without credentials or a fragment")
	}
	return nil
}

func (runner *Runner) profilerJob(request ProfilerRequest) JobFunc {
	return func(ctx context.Context) (evidence map[string]any, runErr error) {
		evidence = map[string]any{"profiler_settings": savedProfilerSettings{URL: request.URL, Target: request.Target}}
		owner := request.SessionID + ":" + request.RunID
		if _, err := runner.store.AcquireLease(request.ProjectID, profilerLeaseResource, owner); err != nil {
			return profilerFailure(evidence, ctx, err)
		}
		defer func() {
			// Restore must still reach the runtime after its capture context is
			// cancelled, but no cleanup call may block a run forever. A failed
			// restore leaves the lease in place to prevent reuse of a tainted
			// diagnostic state until an operator has recovered it.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runner.profilerCleanupTimeout)
			defer cancel()
			if err := runner.profilerRuntime.Restore(cleanupCtx, request); err != nil {
				profilerCleanupFailure(evidence, &runErr, wrapProfilerCleanupError("restore profiler", err))
				return
			}
			if err := runner.store.ReleaseLease(request.ProjectID, profilerLeaseResource, owner); err != nil {
				profilerCleanupFailure(evidence, &runErr, wrapProfilerCleanupError("release profiler lease", err))
			}
		}()

		if err := runner.profilerRuntime.Activate(ctx, request); err != nil {
			return profilerFailure(evidence, ctx, err)
		}
		if err := runner.profilerRuntime.Capture(ctx, request); err != nil {
			return profilerFailure(evidence, ctx, err)
		}
		csv, err := runner.profilerRuntime.Collect(ctx, request)
		if err != nil {
			return profilerFailure(evidence, ctx, err)
		}
		artifactPath, err := runner.store.RunArtifactPath(request.ProjectID, request.SessionID, request.RunID, profilerArtifactRelativePath)
		if err != nil {
			return profilerFailure(evidence, ctx, err)
		}
		if err := os.WriteFile(artifactPath, csv, 0o600); err != nil {
			return profilerFailure(evidence, ctx, fmt.Errorf("write profiler artifact: %w", err))
		}
		sum := sha256.Sum256(csv)
		evidence["artifact"] = Artifact{
			Kind:   "profiler-csv",
			Path:   artifactPath,
			SHA256: hex.EncodeToString(sum[:]),
		}
		return evidence, nil
	}
}

func profilerCleanupFailure(evidence map[string]any, runErr *error, cleanupErr error) {
	if *runErr == nil {
		*runErr = cleanupErr
	} else {
		*runErr = errors.Join(*runErr, cleanupErr)
	}
	evidence["infrastructure_error"] = (*runErr).Error()
}

func profilerFailure(evidence map[string]any, ctx context.Context, err error) (map[string]any, error) {
	if ctx == nil || ctx.Err() == nil {
		evidence["infrastructure_error"] = err.Error()
	}
	return evidence, err
}

func wrapProfilerCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func profilerArtifacts(jobs []JobResult) []Artifact {
	for _, job := range jobs {
		if job.ID != "profiler" {
			continue
		}
		artifact, ok := job.Evidence["artifact"].(Artifact)
		if ok {
			return []Artifact{artifact}
		}
	}
	return nil
}
