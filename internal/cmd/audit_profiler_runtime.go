package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"govard/internal/audit"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

const (
	auditProfilerDockerTimeout = 30 * time.Second
	auditProfilerHTTPTimeout   = 30 * time.Second
	auditProfilerNginxSubdir   = "audit-profiler"
)

var (
	auditProfilerEnvironmentNamePattern  = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	auditProfilerEnvironmentValuePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type auditProfilerRuntimeDependencies struct {
	runDocker func(context.Context, ...string) ([]byte, error)
	httpGet   func(context.Context, string) (int, error)
}

// AuditProfilerRuntimeDependenciesForTest replaces only external Docker and
// HTTP transport. Host-side config lifecycle remains real in unit tests.
type AuditProfilerRuntimeDependenciesForTest struct {
	RunDocker func(context.Context, ...string) ([]byte, error)
	HTTPGet   func(context.Context, string) (int, error)
}

type govardAuditProfilerRuntime struct {
	projectRoot  string
	projectName  string
	webServer    string
	profile      types.AuditProfilerProfile
	runDocker    func(context.Context, ...string) ([]byte, error)
	httpGet      func(context.Context, string) (int, error)
	phpContainer string
	webContainer string
}

func defaultAuditProfilerRuntimeDependencies() auditProfilerRuntimeDependencies {
	client := &http.Client{Timeout: auditProfilerHTTPTimeout}
	return auditProfilerRuntimeDependencies{
		runDocker: runAuditProfilerDocker,
		httpGet: func(ctx context.Context, targetURL string) (int, error) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
			if err != nil {
				return 0, fmt.Errorf("build profiler HTTP request: %w", err)
			}
			// Stock Magento only arms its profiler when HTTP_ACCEPT contains
			// "text/html" (app/bootstrap.php), so the capture must look like a
			// browser navigation rather than a bare API probe.
			request.Header.Set("Accept", "text/html,application/xhtml+xml")
			response, err := client.Do(request)
			if err != nil {
				return 0, fmt.Errorf("capture profiler URL: %w", err)
			}
			defer response.Body.Close()
			// The response body is irrelevant to stock Magento profiling. Reading
			// only a bounded prefix permits connection reuse for ordinary pages
			// without buffering a large catalog response in Govard.
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
			return response.StatusCode, nil
		},
	}
}

func runAuditProfilerDocker(ctx context.Context, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return nil, fmt.Errorf("docker %s: %w", strings.Join(arguments, " "), err)
		}
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(arguments, " "), err, detail)
	}
	return output, nil
}

func newAuditProfilerRuntime(request AuditRunnerRequest, dependencies auditProfilerRuntimeDependencies) (audit.ProfilerRuntime, error) {
	if request.Config == nil || request.Target.Mode == types.AuditTargetStandalone {
		return nil, errors.New("audit profiler does not support standalone targets; run it from a Govard project")
	}
	if request.Target.Mode != types.AuditTargetProject {
		return nil, fmt.Errorf("audit profiler requires a project target, got %q", request.Target.Mode)
	}
	if request.Definition.AuditProfiler == nil {
		return nil, fmt.Errorf("framework %q does not support profiler audit", request.Definition.Name)
	}
	profile := *request.Definition.AuditProfiler
	if err := validateAuditProfilerProfile(profile); err != nil {
		return nil, fmt.Errorf("framework %q audit profiler: %w", request.Definition.Name, err)
	}
	projectRoot := strings.TrimSpace(request.ProjectRoot)
	if projectRoot == "" || !filepath.IsAbs(projectRoot) {
		return nil, errors.New("audit profiler project root must be an absolute path")
	}
	projectName := strings.TrimSpace(request.Config.ProjectName)
	if projectName == "" {
		return nil, errors.New("audit profiler project name is required")
	}
	webServer := strings.ToLower(strings.TrimSpace(request.Config.Stack.Services.WebServer))
	if webServer == "" {
		webServer = strings.ToLower(strings.TrimSpace(request.Config.Stack.WebServer))
	}
	var webContainer string
	switch webServer {
	case "nginx", "apache":
		webContainer = projectName + conventions.WebSuffix
	case "hybrid":
		webContainer = projectName + "-apache" + conventions.ReplicaSuffix
	default:
		return nil, fmt.Errorf("audit profiler does not support web server %q (use nginx, apache, or hybrid)", webServer)
	}
	if dependencies.runDocker == nil || dependencies.httpGet == nil {
		return nil, errors.New("audit profiler runtime dependencies are not configured")
	}
	return &govardAuditProfilerRuntime{
		projectRoot:  filepath.Clean(projectRoot),
		projectName:  projectName,
		webServer:    webServer,
		profile:      profile,
		runDocker:    dependencies.runDocker,
		httpGet:      dependencies.httpGet,
		phpContainer: projectName + conventions.PHPSuffix,
		webContainer: webContainer,
	}, nil
}

func validateAuditProfilerProfile(profile types.AuditProfilerProfile) error {
	if !auditProfilerEnvironmentNamePattern.MatchString(profile.EnvironmentVariable) {
		return fmt.Errorf("invalid FastCGI environment variable %q", profile.EnvironmentVariable)
	}
	if !auditProfilerEnvironmentValuePattern.MatchString(profile.EnvironmentValue) {
		return fmt.Errorf("invalid FastCGI environment value %q", profile.EnvironmentValue)
	}
	cleanOutputPath := path.Clean(strings.TrimSpace(profile.OutputPath))
	if cleanOutputPath == "." || path.IsAbs(cleanOutputPath) || cleanOutputPath == ".." || strings.HasPrefix(cleanOutputPath, "../") {
		return fmt.Errorf("output path %q must stay below the project work directory", profile.OutputPath)
	}
	return nil
}

func (runtime *govardAuditProfilerRuntime) Activate(ctx context.Context, request audit.ProfilerRequest) error {
	if err := runtime.validateRequest(request); err != nil {
		return err
	}
	if err := runtime.removeRuntimeCSV(ctx); err != nil {
		return fmt.Errorf("clear Magento profiler CSV: %w", err)
	}
	configPath, content := runtime.profilerConfig(request)
	if err := writeAuditProfilerConfigAtomic(configPath, []byte(content)); err != nil {
		return fmt.Errorf("write profiler web-server config: %w", err)
	}
	if err := runtime.reloadWebServer(ctx); err != nil {
		return fmt.Errorf("reload %s after enabling profiler: %w", runtime.webServer, err)
	}
	return nil
}

func (runtime *govardAuditProfilerRuntime) Capture(ctx context.Context, request audit.ProfilerRequest) error {
	if err := runtime.validateRequest(request); err != nil {
		return err
	}
	captureCtx, cancel := context.WithTimeout(ctx, auditProfilerHTTPTimeout)
	defer cancel()
	status, err := runtime.httpGet(captureCtx, request.URL)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusBadRequest {
		return fmt.Errorf("capture profiler URL %q returned HTTP %d", request.URL, status)
	}
	return nil
}

func (runtime *govardAuditProfilerRuntime) Collect(ctx context.Context, request audit.ProfilerRequest) ([]byte, error) {
	if err := runtime.validateRequest(request); err != nil {
		return nil, err
	}
	output, err := runtime.docker(ctx, "exec", runtime.phpContainer, "cat", runtime.containerOutputPath())
	if err != nil {
		return nil, fmt.Errorf("collect Magento profiler CSV: %w", err)
	}
	if len(output) == 0 {
		return nil, errors.New("collect Magento profiler CSV: profiler output is empty")
	}
	return output, nil
}

func (runtime *govardAuditProfilerRuntime) Restore(ctx context.Context, request audit.ProfilerRequest) error {
	var restoreErrors []error
	configPath, _ := runtime.profilerConfig(request)
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		restoreErrors = append(restoreErrors, fmt.Errorf("remove profiler web-server config: %w", err))
	}
	if err := runtime.reloadWebServer(ctx); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("reload %s after disabling profiler: %w", runtime.webServer, err))
	}
	if err := runtime.removeRuntimeCSV(ctx); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("remove Magento profiler CSV: %w", err))
	}
	return errors.Join(restoreErrors...)
}

func (runtime *govardAuditProfilerRuntime) validateRequest(request audit.ProfilerRequest) error {
	if request.Target.Mode != types.AuditTargetProject || filepath.Clean(request.ProjectRoot) != runtime.projectRoot {
		return errors.New("audit profiler request is not for the configured Govard project target")
	}
	return audit.ValidateProfilerURL(request.URL)
}

func (runtime *govardAuditProfilerRuntime) profilerConfig(request audit.ProfilerRequest) (string, string) {
	sum := sha256.Sum256([]byte(strings.Join([]string{request.ProjectID, request.SessionID, request.RunID}, "\x00")))
	filename := "govard-audit-profiler-" + hex.EncodeToString(sum[:8]) + ".conf"
	if runtime.webServer == "nginx" {
		configPath := filepath.Join(runtime.projectRoot, engine.ProjectNginxCustomDir, auditProfilerNginxSubdir, filename)
		return configPath, fmt.Sprintf("# Managed temporarily by Govard audit.\nfastcgi_param %s %s;\n", runtime.profile.EnvironmentVariable, runtime.profile.EnvironmentValue)
	}
	configPath := filepath.Join(runtime.projectRoot, engine.ProjectApacheCustomDir, filename)
	return configPath, fmt.Sprintf("# Managed temporarily by Govard audit.\nProxyFCGISetEnvIf \"true\" %s \"%s\"\n", runtime.profile.EnvironmentVariable, runtime.profile.EnvironmentValue)
}

func writeAuditProfilerConfigAtomic(destination string, content []byte) (resultErr error) {
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".govard-audit-profiler-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func (runtime *govardAuditProfilerRuntime) reloadWebServer(ctx context.Context) error {
	if runtime.webServer == "nginx" {
		_, err := runtime.docker(ctx, "exec", runtime.webContainer, "nginx", "-s", "reload")
		return err
	}
	_, err := runtime.docker(ctx, "exec", runtime.webContainer, "httpd", "-k", "graceful")
	return err
}

func (runtime *govardAuditProfilerRuntime) removeRuntimeCSV(ctx context.Context) error {
	_, err := runtime.docker(ctx, "exec", runtime.phpContainer, "rm", "-f", runtime.containerOutputPath())
	return err
}

func (runtime *govardAuditProfilerRuntime) containerOutputPath() string {
	return path.Join(conventions.DefaultWorkDir, path.Clean(runtime.profile.OutputPath))
}

func (runtime *govardAuditProfilerRuntime) docker(ctx context.Context, arguments ...string) ([]byte, error) {
	dockerCtx, cancel := context.WithTimeout(ctx, auditProfilerDockerTimeout)
	defer cancel()
	return runtime.runDocker(dockerCtx, arguments...)
}

// AuditProfilerHTTPGetForTest exposes the default capture transport so tests
// can assert wire-level request properties against an httptest server.
func AuditProfilerHTTPGetForTest() func(context.Context, string) (int, error) {
	return defaultAuditProfilerRuntimeDependencies().httpGet
}

// NewAuditProfilerRuntimeForTest exposes command-boundary selection while
// replacing only external transports; config files are still written and
// removed by the real adapter.
func NewAuditProfilerRuntimeForTest(request AuditRunnerRequest, dependencies AuditProfilerRuntimeDependenciesForTest) (audit.ProfilerRuntime, error) {
	return newAuditProfilerRuntime(request, auditProfilerRuntimeDependencies{
		runDocker: dependencies.RunDocker,
		httpGet:   dependencies.HTTPGet,
	})
}
