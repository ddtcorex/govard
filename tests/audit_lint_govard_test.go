package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/frameworks/types"
)

const testLocalLintImageID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func TestGovardLintBackendMountsReadOnlySourceAndWritableCacheAndOutput(t *testing.T) {
	request := govardLintRequestForTest(t)
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeGovardLintReportForTest(t, request, run, "passed")
		return nil
	}
	report, err := backend.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" {
		t.Fatalf("report status = %q", report.Status)
	}
	runs := docker.RunRequests()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0]
	source := govardLintMountForTest(t, run, "/source")
	if source.Source != request.ProjectRoot || !source.ReadOnly {
		t.Fatalf("source mount = %#v, want read-only project root", source)
	}
	cache := govardLintMountForTest(t, run, "/cache")
	if cache.ReadOnly {
		t.Fatalf("cache mount is read only: %#v", cache)
	}
	output := govardLintMountForTest(t, run, "/output")
	if output.Source != request.RunDir || output.ReadOnly {
		t.Fatalf("output mount = %#v, want writable run directory", output)
	}
	if info, statErr := os.Stat(cache.Source); statErr != nil || !info.IsDir() {
		t.Fatalf("cache directory %q: %v", cache.Source, statErr)
	}
	if run.AutoRemove {
		t.Fatal("lint container requested auto-remove")
	}
	if got, want := run.Image, "govard-local/glint:"; !strings.HasPrefix(got, want) {
		t.Fatalf("image = %q, want the resolved toolchain image", got)
	}
	for _, fragment := range [][]string{
		{"--target-mode", "project"},
		{"--php", "8.2"},
		{"--linter", "phpcs,phpstan"},
		{"--jobs", "2"},
		{"--report", "/output/report.json"},
	} {
		if !containsAdjacent(run.Args, fragment) {
			t.Fatalf("args %#v are missing %#v", run.Args, fragment)
		}
	}
	if containsAdjacent(run.Args, []string{"--target-relative"}) {
		t.Fatalf("project target passed a relative path: %#v", run.Args)
	}
	if containsAdjacent(run.Args, []string{"--no-result-cache"}) {
		t.Fatalf("result cache was bypassed without request: %#v", run.Args)
	}
	for key, want := range map[string]string{
		"GOVARD_LINT_PROVIDER":        "govard",
		"GOVARD_LINT_SESSION_ID":      request.SessionID,
		"GOVARD_LINT_RUN_ID":          request.RunID,
		"GOVARD_LINT_PROJECT_ID":      request.ProjectID,
		"GOVARD_LINT_TARGET_ID":       request.TargetID,
		"GOVARD_LINT_TARGET_MODE":     string(types.AuditTargetProject),
		"GOVARD_LINT_TARGET_PATH":     request.Target.TargetPath,
		"GOVARD_LINT_IMAGE_DIGEST":    testLocalLintImageID,
		"GOVARD_LINT_MATRIX_COMPLETE": "true",
		"GOVARD_LINT_CODING_STANDARD": "Magento2",
		"GOVARD_LINT_PHPSTAN_LEVEL":   "5",
	} {
		if got := run.Environment[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if run.Environment["GOVARD_LINT_TOOLCHAIN_DIGEST"] == "" {
		t.Fatal("toolchain digest was not passed to the runner")
	}
	if run.Environment["GOVARD_LINT_WORKSPACE_DIR"] != "" {
		t.Fatalf("workspace directory was overridden: %q", run.Environment["GOVARD_LINT_WORKSPACE_DIR"])
	}
}

func TestGovardLintBackendPassesNestedModuleRelativePath(t *testing.T) {
	request := govardLintRequestForTest(t)
	relative := filepath.Join("app", "code", "Vendor", "Module")
	module := filepath.Join(request.ProjectRoot, relative)
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	request.Target.Mode = types.AuditTargetModule
	request.Target.TargetPath = module
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeGovardLintReportForTest(t, request, run, "passed")
		return nil
	}
	if _, err := backend.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	run := docker.RunRequests()[0]
	if source := govardLintMountForTest(t, run, "/source"); source.Source != request.ProjectRoot || !source.ReadOnly {
		t.Fatalf("module source mount = %#v, want the read-only project root", source)
	}
	if !containsAdjacent(run.Args, []string{"--target-relative", "app/code/Vendor/Module"}) {
		t.Fatalf("args %#v do not carry the nested relative target", run.Args)
	}
	if !containsAdjacent(run.Args, []string{"--target-mode", "module_in_project"}) {
		t.Fatalf("args %#v do not carry the module target mode", run.Args)
	}
}

func TestGovardLintBackendIsolatesStandaloneWorktree(t *testing.T) {
	request := govardLintRequestForTest(t)
	module := filepath.Join(request.ProjectRoot, "standalone-module")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	request.Target = types.AuditTarget{Framework: "magento2", TargetPath: module, Mode: types.AuditTargetStandalone}
	request.SelectedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4", "8.5"}
	request.MatrixComplete = true
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeGovardLintReportForTest(t, request, run, "passed", "passed", "passed", "passed", "passed")
		return nil
	}
	if _, err := backend.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	run := docker.RunRequests()[0]
	source := govardLintMountForTest(t, run, "/source")
	if source.Source != module || !source.ReadOnly {
		t.Fatalf("standalone source mount = %#v, want the read-only module itself", source)
	}
	if containsAdjacent(run.Args, []string{"--target-relative"}) {
		t.Fatalf("standalone target passed a relative path: %#v", run.Args)
	}
	if !containsAdjacent(run.Args, []string{"--target-mode", "standalone"}) {
		t.Fatalf("args %#v do not carry the standalone target mode", run.Args)
	}
	if !containsAdjacent(run.Args, []string{"--php", "8.1,8.2,8.3,8.4,8.5"}) {
		t.Fatalf("args %#v do not carry the standalone matrix", run.Args)
	}
}

func TestGovardLintBackendMountsComposerAuthReadOnlyOnlyWhenConfigured(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"http-basic":{"repo.magento.com":{"username":"key","password":"secret"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name string
		auth string
		want bool
	}{
		{name: "configured", auth: authPath, want: true},
		{name: "absent", auth: "", want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, func(options *audit.GovardLintOptions) {
				options.AuthJSON = testCase.auth
			})
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeGovardLintReportForTest(t, request, run, "passed")
				return nil
			}
			if _, err := backend.Run(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			run := docker.RunRequests()[0]
			mount, found := govardLintOptionalMountForTest(run, "/auth/auth.json")
			if found != testCase.want {
				t.Fatalf("auth mount present = %t, want %t", found, testCase.want)
			}
			if !testCase.want {
				return
			}
			if mount.Source != authPath || !mount.ReadOnly {
				t.Fatalf("auth mount = %#v, want a read-only host auth.json", mount)
			}
			for _, argument := range run.Args {
				if strings.Contains(argument, authPath) {
					t.Fatalf("auth path leaked into arguments: %#v", run.Args)
				}
			}
			for key, value := range run.Environment {
				if strings.Contains(value, authPath) || strings.Contains(value, "secret") {
					t.Fatalf("auth material leaked into environment %s=%q", key, value)
				}
			}
			log, err := os.ReadFile(filepath.Join(request.RunDir, "govard-lint.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(log), "secret") || strings.Contains(string(log), authPath) {
				t.Fatalf("auth material leaked into the lint log: %q", log)
			}
		})
	}
}

func TestGovardLintBackendForwardsSSHAgentOnlyWhenAllowed(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	for _, testCase := range []struct {
		name   string
		allow  bool
		socket string
		want   bool
	}{
		{name: "opted in", allow: true, socket: socket, want: true},
		{name: "not allowed", allow: false, socket: socket, want: false},
		{name: "allowed without socket", allow: true, socket: "", want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, func(options *audit.GovardLintOptions) {
				options.AllowSSHAgent = testCase.allow
				options.SSHAgent = testCase.socket
			})
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeGovardLintReportForTest(t, request, run, "passed")
				return nil
			}
			if _, err := backend.Run(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			run := docker.RunRequests()[0]
			mount, found := govardLintOptionalMountForTest(run, "/ssh-agent")
			if found != testCase.want {
				t.Fatalf("ssh agent mount present = %t, want %t", found, testCase.want)
			}
			if got, wantSocket := run.Environment["SSH_AUTH_SOCK"], "/ssh-agent"; testCase.want != (got == wantSocket) {
				t.Fatalf("SSH_AUTH_SOCK = %q, want forwarded = %t", got, testCase.want)
			}
			if testCase.want && mount.Source != testCase.socket {
				t.Fatalf("ssh agent mount = %#v, want host socket %q", mount, testCase.socket)
			}
			if testCase.want && mount.ReadOnly {
				t.Fatal("ssh agent socket was mounted read only")
			}
		})
	}
}

func TestGovardLintBackendRunsAsHostUserAndLabelsSessionAndRun(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		uid       int
		gid       int
		allowRoot bool
		want      string
	}{
		{name: "host user", uid: 1000, gid: 1000, want: "1000:1000"},
		{name: "explicit root", uid: 0, gid: 0, allowRoot: true, want: "0:0"},
		{name: "deliberately unset user", uid: -1, gid: -1, allowRoot: true, want: ""},
		{name: "user without group", uid: 501, gid: -1, allowRoot: true, want: "501"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, func(options *audit.GovardLintOptions) {
				options.UID = testCase.uid
				options.GID = testCase.gid
				options.AllowRootUser = testCase.allowRoot
			})
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeGovardLintReportForTest(t, request, run, "passed")
				return nil
			}
			if _, err := backend.Run(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			run := docker.RunRequests()[0]
			if run.User != testCase.want {
				t.Fatalf("container user = %q, want %q", run.User, testCase.want)
			}
			for key, want := range map[string]string{
				"io.govard.audit.session": request.SessionID,
				"io.govard.audit.run":     request.RunID,
			} {
				if got := run.Labels[key]; got != want {
					t.Fatalf("label %s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestGovardLintBackendBoundsAnalyzerJobs(t *testing.T) {
	for _, testCase := range []struct {
		name string
		jobs int
		want string
	}{
		{name: "below range", jobs: 0, want: "1"},
		{name: "in range", jobs: 4, want: "4"},
		{name: "above range", jobs: 512, want: "32"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			request.Jobs = testCase.jobs
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, nil)
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeGovardLintReportForTest(t, request, run, "passed")
				return nil
			}
			if _, err := backend.Run(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if !containsAdjacent(docker.RunRequests()[0].Args, []string{"--jobs", testCase.want}) {
				t.Fatalf("args %#v do not bound jobs to %q", docker.RunRequests()[0].Args, testCase.want)
			}
		})
	}
}

func TestGovardLintBackendInterpretsRunnerExitCodes(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		exitCode  int
		outcome   string
		status    string
		wantError string
	}{
		{name: "passed", exitCode: 0, outcome: "passed", status: "passed"},
		{name: "findings", exitCode: 1, outcome: "failed", status: "failed"},
		{name: "unsupported", exitCode: 3, outcome: "unsupported", status: "unsupported"},
		{name: "infrastructure", exitCode: 2, outcome: "infra_error", status: "infra_error"},
		{name: "cancelled by signal", exitCode: 143, outcome: "cancelled", status: "cancelled"},
		{name: "usage error", exitCode: 64, outcome: "passed", wantError: "usage"},
		{name: "docker failure", exitCode: 125, outcome: "passed", wantError: "infrastructure exit code 125"},
		{name: "status disagrees with exit code", exitCode: 0, outcome: "failed", wantError: "does not match"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, nil)
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeGovardLintReportForTest(t, request, run, testCase.outcome)
				return commandExitErrorForTest(t, testCase.exitCode)
			}
			report, err := backend.Run(context.Background(), request)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error = %v, want one mentioning %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != testCase.status {
				t.Fatalf("report status = %q, want %q", report.Status, testCase.status)
			}
			if len(report.PHPResults) != 1 || report.PHPResults[0].Outcome != testCase.outcome {
				t.Fatalf("php results = %#v", report.PHPResults)
			}
			docker.mu.Lock()
			removes := len(docker.removes)
			docker.mu.Unlock()
			if removes != 1 {
				t.Fatalf("completed container removes = %d, want 1", removes)
			}
		})
	}
}

func TestGovardLintBackendQuarantinesMalformedOrMismatchedReports(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		corrupt func(*testing.T, audit.LintRequest, audit.ContainerRunRequest)
	}{
		{
			name: "malformed report",
			corrupt: func(t *testing.T, request audit.LintRequest, _ audit.ContainerRunRequest) {
				if err := os.WriteFile(filepath.Join(request.RunDir, "report.json"), []byte("{not json"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mismatched project identity",
			corrupt: func(t *testing.T, request audit.LintRequest, run audit.ContainerRunRequest) {
				report := govardLintReportForTest(request, run, "passed")
				report.ProjectID = "project-other"
				writeLintReportFileForTest(t, request.RunDir, report)
			},
		},
		{
			name: "mismatched toolchain digest",
			corrupt: func(t *testing.T, request audit.LintRequest, run audit.ContainerRunRequest) {
				report := govardLintReportForTest(request, run, "passed")
				report.ToolchainDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
				writeLintReportFileForTest(t, request.RunDir, report)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := govardLintRequestForTest(t)
			docker := &fakeLintDocker{}
			backend := newGovardLintBackendForTest(t, docker, nil)
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				testCase.corrupt(t, request, run)
				return nil
			}
			_, err := backend.Run(context.Background(), request)
			if err == nil {
				t.Fatal("an unusable lint report was accepted")
			}
			if !strings.Contains(err.Error(), "quarantine") {
				t.Fatalf("error = %v, want a quarantine diagnostic", err)
			}
			if !regexp.MustCompile(`sha256:[0-9a-f]{64}`).MatchString(err.Error()) {
				t.Fatalf("error = %v, want the quarantined report digest", err)
			}
			if strings.Contains(err.Error(), "not json") || strings.Contains(err.Error(), "project-other") {
				t.Fatalf("error leaked quarantined report content: %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(request.RunDir, "report.json")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unusable report was left in place: %v", statErr)
			}
			entries, readErr := os.ReadDir(filepath.Join(request.RunDir, "quarantine"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 1 {
				t.Fatalf("quarantine entries = %#v, want exactly one", entries)
			}
			info, statErr := os.Stat(filepath.Join(request.RunDir, "quarantine", entries[0].Name()))
			if statErr != nil {
				t.Fatal(statErr)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("quarantined report mode = %v, want 0600", info.Mode().Perm())
			}
			if !strings.Contains(err.Error(), filepath.Join(request.RunDir, "quarantine", entries[0].Name())) {
				t.Fatalf("error = %v, want the quarantined report path", err)
			}
		})
	}
}

func TestGovardLintBackendRemovesStaleReportBeforeRun(t *testing.T) {
	request := govardLintRequestForTest(t)
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	stale := audit.LintReport{SchemaVersion: audit.LintReportSchemaVersion, Provider: request.Provider, SessionID: request.SessionID, RunID: request.RunID, ProjectID: request.ProjectID, TargetID: request.TargetID, TargetMode: request.Target.Mode, TargetPath: request.Target.TargetPath, ImageDigest: testLocalLintImageID, ToolchainDigest: "sha256:stale", Status: "passed", SelectedPHPVersions: request.PHPVersions(), MatrixComplete: request.MatrixComplete, PHPResults: []audit.LintPHPResult{{PHPVersion: "8.2", Outcome: "passed", Cache: audit.CacheOutcome{State: "warm"}, Phases: []audit.LintPhase{{Name: "phpcs", Status: "passed"}}}}}
	writeLintReportFileForTest(t, request.RunDir, stale)
	docker.run = func(_ context.Context, _ audit.ContainerRunRequest, _ io.Writer) error { return nil }
	if _, err := backend.Run(context.Background(), request); err == nil {
		t.Fatal("a stale report from a previous run was accepted as this run's result")
	}
	if _, err := os.Stat(filepath.Join(request.RunDir, "report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale report was not removed: %v", err)
	}
}

func TestGovardLintBackendKeepsRemainingPHPResultsForCompatibilityFinding(t *testing.T) {
	request := govardLintRequestForTest(t)
	request.Target = types.AuditTarget{Framework: "magento2", TargetPath: request.ProjectRoot, Mode: types.AuditTargetStandalone}
	request.SelectedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4", "8.5"}
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeGovardLintReportForTest(t, request, run, "passed", "failed", "passed", "passed", "passed")
		return commandExitErrorForTest(t, 1)
	}
	report, err := backend.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" {
		t.Fatalf("aggregate status = %q, want failed", report.Status)
	}
	if len(report.PHPResults) != 5 {
		t.Fatalf("php results = %d, want every requested version", len(report.PHPResults))
	}
	if report.PHPResults[1].Outcome != "failed" || len(report.PHPResults[1].Findings) == 0 {
		t.Fatalf("compatibility result = %#v", report.PHPResults[1])
	}
	for _, index := range []int{0, 2, 3, 4} {
		if report.PHPResults[index].Outcome != "passed" {
			t.Fatalf("php %s stopped after a compatibility finding: %#v", report.PHPResults[index].PHPVersion, report.PHPResults[index])
		}
	}
}

func TestGovardLintBackendCancellationStopsThenRemovesContainer(t *testing.T) {
	request := govardLintRequestForTest(t)
	docker := &fakeLintDocker{started: make(chan struct{})}
	var cleanupOrder []string
	var stopTimeout time.Duration
	docker.stop = func(_ context.Context, _ string, timeout time.Duration) error {
		docker.mu.Lock()
		cleanupOrder = append(cleanupOrder, "stop")
		stopTimeout = timeout
		docker.mu.Unlock()
		return nil
	}
	docker.remove = func(context.Context, string) error {
		docker.mu.Lock()
		cleanupOrder = append(cleanupOrder, "remove")
		docker.mu.Unlock()
		return nil
	}
	docker.run = func(ctx context.Context, _ audit.ContainerRunRequest, _ io.Writer) error {
		close(docker.started)
		<-ctx.Done()
		return ctx.Err()
	}
	backend := newGovardLintBackendForTest(t, docker, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		report audit.LintReport
		err    error
	}, 1)
	go func() {
		report, err := backend.Run(ctx, request)
		done <- struct {
			report audit.LintReport
			err    error
		}{report: report, err: err}
	}()
	<-docker.started
	cancel()
	select {
	case result := <-done:
		if result.err == nil || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("cancellation error = %v", result.err)
		}
		if result.report.Status != string(audit.StatusCancelled) {
			t.Fatalf("cancelled report status = %q", result.report.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lint backend did not return after cancellation")
	}
	docker.mu.Lock()
	order := append([]string(nil), cleanupOrder...)
	timeout := stopTimeout
	docker.mu.Unlock()
	if !reflect.DeepEqual(order, []string{"stop", "remove"}) {
		t.Fatalf("cleanup order = %#v, want stop then remove", order)
	}
	if timeout != 5*time.Second {
		t.Fatalf("stop timeout = %s, want 5s", timeout)
	}
}

func TestGovardLintBackendPreservesCancellationBeforeContainerExecution(t *testing.T) {
	request := govardLintRequestForTest(t)
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	cause := errors.New("audit session superseded")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	report, err := backend.Run(ctx, request)
	if !errors.Is(err, cause) || report.Status != string(audit.StatusCancelled) {
		t.Fatalf("report/error = %#v/%v", report, err)
	}
	docker.mu.Lock()
	runs, stops, removes := len(docker.runs), len(docker.stops), len(docker.removes)
	docker.mu.Unlock()
	if runs != 0 || stops != 0 || removes != 0 {
		t.Fatalf("container calls = run:%d stop:%d remove:%d, want none", runs, stops, removes)
	}
}

func TestGovardLintBackendBypassesResultCacheOnRequest(t *testing.T) {
	t.Run("bypass is passed and reported", func(t *testing.T) {
		request := govardLintRequestForTest(t)
		request.BypassResultCache = true
		docker := &fakeLintDocker{}
		backend := newGovardLintBackendForTest(t, docker, nil)
		docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
			if !containsAdjacent(run.Args, []string{"--no-result-cache"}) {
				t.Fatalf("args %#v do not bypass the result cache", run.Args)
			}
			writeGovardLintReportForTest(t, request, run, "passed")
			return nil
		}
		report, err := backend.Run(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if report.PHPResults[0].Cache.State != "bypassed" {
			t.Fatalf("cache state = %#v, want bypassed", report.PHPResults[0].Cache)
		}
	})

	t.Run("a warm cache contradicts the bypass request", func(t *testing.T) {
		request := govardLintRequestForTest(t)
		request.BypassResultCache = true
		docker := &fakeLintDocker{}
		backend := newGovardLintBackendForTest(t, docker, nil)
		docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
			report := govardLintReportForTest(request, run, "passed")
			report.PHPResults[0].Cache.State = "warm"
			writeLintReportFileForTest(t, request.RunDir, report)
			return nil
		}
		if _, err := backend.Run(context.Background(), request); err == nil {
			t.Fatal("a warm cache was accepted while the result cache was bypassed")
		}
	})

	t.Run("an unknown cache state is rejected", func(t *testing.T) {
		request := govardLintRequestForTest(t)
		docker := &fakeLintDocker{}
		backend := newGovardLintBackendForTest(t, docker, nil)
		docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
			report := govardLintReportForTest(request, run, "passed")
			report.PHPResults[0].Cache.State = "hot"
			writeLintReportFileForTest(t, request.RunDir, report)
			return nil
		}
		if _, err := backend.Run(context.Background(), request); err == nil {
			t.Fatal("an unknown cache state was accepted")
		}
	})
}

func TestGovardLintBackendKeysCacheGenerationOnToolchainWithoutExposingPaths(t *testing.T) {
	base := govardLintRequestForTest(t)
	if err := os.WriteFile(filepath.Join(base.ProjectRoot, "composer.lock"), []byte(`{"packages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	first := govardLintCacheDirForTest(t, base)
	if got := filepath.Dir(first); got != filepath.Join(base.CacheRoot, base.TargetID) {
		t.Fatalf("cache directory parent = %q, want the per-target namespace", got)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(filepath.Base(first)) {
		t.Fatalf("cache generation %q is not an opaque digest", filepath.Base(first))
	}
	if strings.Contains(first, base.ProjectRoot) {
		t.Fatalf("cache directory %q exposes the literal source path %q", first, base.ProjectRoot)
	}
	if second := govardLintCacheDirForTest(t, base); second != first {
		t.Fatalf("cache directory changed between identical runs: %q vs %q", first, second)
	}
	inputs := govardLintCacheInputsForTest(t, first)
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(inputs) {
		t.Fatalf("recorded cache inputs = %q, want an opaque digest", inputs)
	}

	// A changed PHP matrix or analyzer policy is a different toolchain, so it
	// gets its own generation.
	changedMatrix := base
	changedMatrix.SelectedPHPVersions = []string{"8.3"}
	if got := govardLintCacheDirForTest(t, changedMatrix); got == first {
		t.Fatal("cache generation ignored the selected PHP matrix")
	}
	changedLinters := base
	changedLinters.Profile.Linters = []string{"phpcs"}
	if got := govardLintCacheDirForTest(t, changedLinters); got == first {
		t.Fatal("cache generation ignored the selected linters")
	}

	changedTarget := base
	changedTarget.TargetID = "target-ffffffffffffffffffffffffffffffff"
	if got := filepath.Dir(govardLintCacheDirForTest(t, changedTarget)); got == filepath.Dir(first) {
		t.Fatal("cache namespace ignored the resolved target identity")
	}
}

// A changed composer.lock must not cost a full cold Composer re-download, and
// must not orphan the previous cache subtree: the generation stays, the
// analyzer state is discarded, and the Composer download cache survives.
func TestGovardLintBackendKeepsComposerCacheAcrossProjectInputChanges(t *testing.T) {
	base := govardLintRequestForTest(t)
	if err := os.WriteFile(filepath.Join(base.ProjectRoot, "composer.lock"), []byte(`{"packages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	generation := govardLintCacheDirForTest(t, base)
	before := govardLintCacheInputsForTest(t, generation)

	for _, testCase := range []struct {
		name  string
		write func(*testing.T, string)
	}{
		{name: "composer.lock", write: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{"packages":[{"name":"vendor/pkg"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "phpcs ruleset", write: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "phpcs.xml"), []byte(`<ruleset name="Local"/>`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "phpstan configuration", write: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "phpstan.neon"), []byte("parameters:\n\tlevel: 8\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			// Stand in for what the runner leaves behind inside /cache.
			analyzerState := filepath.Join(generation, "analyzer", "php-8.2", "cache-key", "phpstan")
			composerState := filepath.Join(generation, "composer", "files", "vendor", "pkg.zip")
			for _, path := range []string{analyzerState, composerState} {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("state"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			testCase.write(t, base.ProjectRoot)

			if got := govardLintCacheDirForTest(t, base); got != generation {
				t.Fatalf("cache generation moved to %q after a project input change, orphaning %q", got, generation)
			}
			if after := govardLintCacheInputsForTest(t, generation); after == before {
				t.Fatal("recorded cache inputs ignored a changed project input")
			} else {
				before = after
			}
			if _, err := os.Stat(analyzerState); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stale analyzer state survived a project input change: %v", err)
			}
			if _, err := os.Stat(composerState); err != nil {
				t.Fatalf("Composer download cache was discarded on a project input change: %v", err)
			}
		})
	}

	// Nothing changed: the analyzer state must now be kept as well.
	analyzerState := filepath.Join(generation, "analyzer", "php-8.2", "cache-key", "phpstan")
	if err := os.MkdirAll(filepath.Dir(analyzerState), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(analyzerState, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := govardLintCacheDirForTest(t, base); got != generation {
		t.Fatalf("cache generation = %q, want the unchanged %q", got, generation)
	}
	if _, err := os.Stat(analyzerState); err != nil {
		t.Fatalf("warm analyzer state was discarded without any input change: %v", err)
	}
	info, err := os.Stat(filepath.Join(generation, ".govard-lint-inputs"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("recorded cache inputs mode = %v, want 0600", info.Mode().Perm())
	}
}

// Nothing else prunes reusable lint caches (Runner.CleanupOlderThan only
// removes persisted sessions), so superseded generations must not accumulate
// forever - while a generation still in active use is left alone.
func TestGovardLintBackendPrunesSupersededCacheGenerations(t *testing.T) {
	base := govardLintRequestForTest(t)
	namespace := filepath.Join(base.CacheRoot, base.TargetID)
	stale := map[string]time.Duration{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": 96 * time.Hour,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": 72 * time.Hour,
		"cccccccccccccccccccccccccccccccc": 48 * time.Hour,
		"dddddddddddddddddddddddddddddddd": 24 * time.Hour,
	}
	recent := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	for name, age := range stale {
		govardLintSeedCacheGenerationForTest(t, namespace, name, time.Now().Add(-age))
	}
	govardLintSeedCacheGenerationForTest(t, namespace, recent, time.Now().Add(-time.Minute))

	current := govardLintCacheDirForTest(t, base)
	entries, err := os.ReadDir(namespace)
	if err != nil {
		t.Fatal(err)
	}
	remaining := make(map[string]bool, len(entries))
	for _, entry := range entries {
		remaining[entry.Name()] = true
	}
	if !remaining[filepath.Base(current)] {
		t.Fatalf("the current generation %q was pruned", filepath.Base(current))
	}
	if !remaining[recent] {
		t.Fatalf("a generation in active use was pruned: %#v", remaining)
	}
	// The two most recently used stale generations are kept; the older two go.
	if !remaining["dddddddddddddddddddddddddddddddd"] || !remaining["cccccccccccccccccccccccccccccccc"] {
		t.Fatalf("recently used generations were pruned: %#v", remaining)
	}
	if remaining["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] || remaining["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] {
		t.Fatalf("superseded generations were orphaned: %#v", remaining)
	}
}

func govardLintSeedCacheGenerationForTest(t *testing.T, namespace, name string, used time.Time) {
	t.Helper()
	directory := filepath.Join(namespace, name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, ".govard-lint-inputs")
	if err := os.WriteFile(marker, []byte("sha256:seeded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(marker, used, used); err != nil {
		t.Fatal(err)
	}
}

func govardLintCacheInputsForTest(t *testing.T, generation string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(generation, ".govard-lint-inputs"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}

func TestGovardLintBackendWritesPrivateRunLog(t *testing.T) {
	request := govardLintRequestForTest(t)
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, output io.Writer) error {
		_, _ = io.WriteString(output, "glint: report passed\n")
		writeGovardLintReportForTest(t, request, run, "passed")
		return nil
	}
	if _, err := backend.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(request.RunDir, "govard-lint.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lint log mode = %v, want 0600", info.Mode().Perm())
	}
	content, err := os.ReadFile(filepath.Join(request.RunDir, "govard-lint.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "glint: report passed") {
		t.Fatalf("lint log = %q, want the container output", content)
	}
}

// The lint image declares no USER, so a container without an explicit host
// user runs as root and publishes a mode-0600 report this process cannot read.
// That has to be an explicit choice, never the result of an unset field.
func TestGovardLintBackendRequiresAnExplicitHostUser(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		uid       int
		gid       int
		allowRoot bool
		wantError bool
	}{
		{name: "zero value is not an opt-in to root", uid: 0, gid: 0, wantError: true},
		{name: "negative user is not an opt-in either", uid: -1, gid: -1, wantError: true},
		{name: "missing group", uid: 1000, gid: 0, wantError: true},
		{name: "explicit root", uid: 0, gid: 0, allowRoot: true},
		{name: "explicit unset user", uid: -1, gid: -1, allowRoot: true},
		{name: "real host user", uid: 1000, gid: 1000},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			docker := &fakeLintDocker{}
			_, err := audit.NewGovardLintBackend(audit.GovardLintOptions{
				Toolchain:     audit.NewToolchainManager(docker, t.TempDir()),
				Docker:        docker,
				UID:           testCase.uid,
				GID:           testCase.gid,
				AllowRootUser: testCase.allowRoot,
			})
			if testCase.wantError {
				if err == nil {
					t.Fatalf("uid %d gid %d was accepted without an explicit root opt-in", testCase.uid, testCase.gid)
				}
				if !strings.Contains(err.Error(), "host user") || !strings.Contains(err.Error(), "AllowRootUser") {
					t.Fatalf("error = %v, want an actionable host-identity message", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGovardLintBackendRejectsForeignProviderAndMissingToolchain(t *testing.T) {
	if _, err := audit.NewGovardLintBackend(audit.GovardLintOptions{}); err == nil {
		t.Fatal("a backend without a toolchain manager was accepted")
	}
	request := govardLintRequestForTest(t)
	request.Provider = "team-ci"
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	if _, err := backend.Run(context.Background(), request); err == nil {
		t.Fatal("a request for another provider was accepted")
	}
	if backend.Name() != "govard" {
		t.Fatalf("backend name = %q, want govard", backend.Name())
	}
}

func TestDefaultLintCacheRootIsDerivedFromGovardHome(t *testing.T) {
	if got, want := audit.DefaultLintCacheRoot(filepath.Join("/home", "user", ".govard")), filepath.Join("/home", "user", ".govard", "cache", "audit", "lint"); got != want {
		t.Fatalf("lint cache root = %q, want %q", got, want)
	}
}

func govardLintCacheDirForTest(t *testing.T, request audit.LintRequest) string {
	t.Helper()
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		outcomes := make([]string, len(request.SelectedPHPVersions))
		for index := range outcomes {
			outcomes[index] = "passed"
		}
		writeGovardLintReportForTest(t, request, run, outcomes...)
		return nil
	}
	if _, err := backend.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	return govardLintMountForTest(t, docker.RunRequests()[0], "/cache").Source
}

func newGovardLintBackendForTest(t *testing.T, docker *fakeLintDocker, mutate func(*audit.GovardLintOptions)) *audit.GovardLintBackend {
	t.Helper()
	if docker.inspect == nil {
		docker.inspect = func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{ID: testLocalLintImageID}, nil
		}
	}
	options := audit.GovardLintOptions{Toolchain: audit.NewToolchainManager(docker, t.TempDir()), Docker: docker, UID: 1000, GID: 1000}
	if mutate != nil {
		mutate(&options)
	}
	backend, err := audit.NewGovardLintBackend(options)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func govardLintRequestForTest(t *testing.T) audit.LintRequest {
	t.Helper()
	root := t.TempDir()
	state := t.TempDir()
	runDir := filepath.Join(state, "runs", "run-0001")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return audit.LintRequest{
		ProjectRoot:         root,
		ProjectID:           "project-aabbccdd",
		SessionID:           "20260816T000000Z-01020304",
		RunID:               "run-0001",
		Provider:            "govard",
		TargetID:            "target-aabbccddeeff00112233445566778899",
		Target:              types.AuditTarget{Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject},
		RunDir:              runDir,
		CacheRoot:           filepath.Join(state, "cache"),
		Scope:               audit.ScopeProject,
		Jobs:                2,
		Profile:             types.AuditLintProfile{ProjectPHPVersions: []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}, StandalonePHPVersions: []string{"8.1", "8.2", "8.3", "8.4", "8.5"}, Linters: []string{"phpcs", "phpstan"}, CodingStandard: "Magento2", PHPStanLevel: 5},
		SelectedPHPVersions: []string{"8.2"},
		MatrixComplete:      true,
	}
}

func govardLintMountForTest(t *testing.T, run audit.ContainerRunRequest, target string) audit.ContainerMount {
	t.Helper()
	mount, found := govardLintOptionalMountForTest(run, target)
	if !found {
		t.Fatalf("mount %q is missing from %#v", target, run.Mounts)
	}
	return mount
}

func govardLintOptionalMountForTest(run audit.ContainerRunRequest, target string) (audit.ContainerMount, bool) {
	for _, mount := range run.Mounts {
		if mount.Target == target {
			return mount, true
		}
	}
	return audit.ContainerMount{}, false
}

func writeGovardLintReportForTest(t *testing.T, request audit.LintRequest, run audit.ContainerRunRequest, outcomes ...string) audit.LintReport {
	t.Helper()
	report := govardLintReportForTest(request, run, outcomes...)
	writeLintReportFileForTest(t, request.RunDir, report)
	return report
}

func govardLintReportForTest(request audit.LintRequest, run audit.ContainerRunRequest, outcomes ...string) audit.LintReport {
	versions := request.PHPVersions()
	cacheState := "cold"
	if containsAdjacent(run.Args, []string{"--no-result-cache"}) {
		cacheState = "bypassed"
	}
	report := audit.LintReport{
		SchemaVersion:       audit.LintReportSchemaVersion,
		Provider:            request.Provider,
		SessionID:           request.SessionID,
		RunID:               request.RunID,
		ProjectID:           request.ProjectID,
		TargetID:            request.TargetID,
		TargetMode:          request.Target.Mode,
		TargetPath:          request.Target.TargetPath,
		ImageDigest:         run.Environment["GOVARD_LINT_IMAGE_DIGEST"],
		ToolchainDigest:     run.Environment["GOVARD_LINT_TOOLCHAIN_DIGEST"],
		DurationMS:          7,
		SelectedPHPVersions: versions,
		MatrixComplete:      request.MatrixComplete,
	}
	for index, version := range versions {
		outcome := "passed"
		if index < len(outcomes) {
			outcome = outcomes[index]
		}
		result := audit.LintPHPResult{
			PHPVersion: version,
			Outcome:    outcome,
			DurationMS: 3,
			Cache:      audit.CacheOutcome{State: cacheState, Key: "key-" + version, Reason: "test fixture"},
			Phases:     []audit.LintPhase{{Name: "phpcs", PHPVersion: version, Status: lintPhaseStatusForTest(outcome), DurationMS: 3, CacheState: cacheState, CacheKey: "key-" + version}},
		}
		if outcome == "failed" {
			result.Findings = []audit.LintFinding{{Tool: "phpcs", Rule: "M2-LINT-COMPAT", Path: "Model/Example.php", Line: 12, Message: "syntax is not supported in PHP " + version}}
		}
		report.PHPResults = append(report.PHPResults, result)
	}
	report.Status = lintAggregateStatusForTest(report.PHPResults)
	return report
}

func lintPhaseStatusForTest(outcome string) string {
	switch outcome {
	case "infra_error":
		return "error"
	default:
		return outcome
	}
}

func lintAggregateStatusForTest(results []audit.LintPHPResult) string {
	var cancelled, infra, failed, passed bool
	for _, result := range results {
		switch result.Outcome {
		case "cancelled":
			cancelled = true
		case "infra_error":
			infra = true
		case "failed":
			failed = true
		case "passed":
			passed = true
		}
	}
	switch {
	case cancelled:
		return "cancelled"
	case infra:
		return "infra_error"
	case failed:
		return "failed"
	case passed:
		return "passed"
	default:
		return "unsupported"
	}
}

func TestLintGovardFallsBackToPSR12WhenStandardMissing(t *testing.T) {
	request := govardLintRequestForTest(t)
	request.Profile.CodingStandard = "WordPress"
	docker := &fakeLintDocker{}
	backend := newGovardLintBackendForTest(t, docker, nil)
	callCount := 0
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, output io.Writer) error {
		callCount++
		if callCount == 1 {
			if got := run.Environment["GOVARD_LINT_CODING_STANDARD"]; got != "WordPress" {
				t.Fatalf("first run coding standard = %q, want WordPress", got)
			}
			if output != nil {
				_, _ = io.WriteString(output, "ERROR: the \"WordPress\" coding standard was not installed.")
			}
			return errors.New("ERROR: the \"WordPress\" coding standard was not installed. Was not installed")
		}
		if got := run.Environment["GOVARD_LINT_CODING_STANDARD"]; got != "PSR12" {
			t.Fatalf("retry coding standard = %q, want PSR12", got)
		}
		writeGovardLintReportForTest(t, request, run, "passed")
		return nil
	}
	report, err := backend.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if report.Status != "passed" {
		t.Fatalf("report status = %q, want passed", report.Status)
	}
	if callCount != 2 {
		t.Fatalf("runs = %d, want 2 (original + fallback)", callCount)
	}
	runs := docker.RunRequests()
	if len(runs) != 2 {
		t.Fatalf("run requests = %d, want 2", len(runs))
	}
	if runs[1].Environment["GOVARD_LINT_CODING_STANDARD"] != "PSR12" {
		t.Fatalf("second run env = %q, want PSR12", runs[1].Environment["GOVARD_LINT_CODING_STANDARD"])
	}
}

func writeLintReportFileForTest(t *testing.T, runDir string, report audit.LintReport) {
	t.Helper()
	content, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPartitionToolingLimitationFindings(t *testing.T) {
	report := audit.LintReport{PHPResults: []audit.LintPHPResult{{
		PHPVersion: "8.4",
		Findings: []audit.LintFinding{
			{Tool: "M2-LINT-COMPAT", Path: "dev/tests/integration/testsuite/Magento/Framework/Jwt/JwtManagerTest.php", Message: "Function openssl_free_key() is deprecated since PHP 8.0"},
			{Tool: "M2-LINT-COMPAT", Message: "Internal error: Magento\\TestModuleExtensionAttributes\\Api\\Data\\FakeRegionInterface does not exist and has no extension interface while analysing file /source/dev/tests/integration/_files/Magento/TestModuleExtensionAttributes/Model/Data/FakeRegion.php"},
			{Tool: "M2-LINT-PHPCS", Rule: "Generic.Files.LineLength.TooLong", Path: "app/code/Acme/Elist/Controller/Submit/Create.php", Message: "Line exceeds 120 characters"},
		},
	}}}
	actionable, limited := audit.PartitionToolingLimitationsForTest(report.PHPResults[0].Findings)
	if len(actionable) != 1 || len(limited) != 2 {
		t.Fatalf("got %d actionable, %d limited; want 1, 2", len(actionable), len(limited))
	}
}

func TestPartitionedLimitationsRecomputeOutcome(t *testing.T) {
	// Partitioning tooling limitations out of a PHP result must leave the
	// actionable findings in place and recompute the outcome: failed while
	// actionable findings remain, passed when only limitations were present.
	report := audit.LintReport{Status: "failed", PHPResults: []audit.LintPHPResult{{
		PHPVersion: "8.4",
		Outcome:    "failed",
		Findings: []audit.LintFinding{
			{Tool: "M2-LINT-COMPAT", Message: "Internal error: FakeRegionInterface does not exist and has no extension interface while analysing file /source/dev/tests/integration/_files/FakeRegion.php"},
			{Tool: "M2-LINT-PHPSTAN", Rule: "method.notFound", Path: "app/code/Acme/Contact/ViewModel/ContactViewModel.php", Line: 56, Message: "Call to an undefined method Magento\\Framework\\View\\Element\\BlockInterface::setData()."},
		},
	}}}
	limited := audit.ApplyToolingLimitationPartitionForTest(&report)
	if limited != 1 {
		t.Fatalf("partitioned %d limited findings; want 1", limited)
	}
	kept := report.PHPResults[0].Findings
	if len(kept) != 1 || kept[0].Rule != "method.notFound" {
		t.Fatalf("kept findings = %+v; want only the actionable method.notFound", kept)
	}
	if report.PHPResults[0].Outcome != "failed" {
		t.Fatalf("outcome = %q; want failed while actionable findings remain", report.PHPResults[0].Outcome)
	}
}
