package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

func TestLintToolchainDigestIncludesExternalCommand(t *testing.T) {
	base := audit.LintToolchain{Provider: "team-ci", Image: "sha256:aaaaaaaa", Command: []string{"/usr/local/bin/glint", "--report-json", "/output/report.json"}, PHPVersions: []string{"8.1", "8.2"}, Linters: []string{"phpcs", "phpstan"}, PHPStanLevel: 5}
	first := audit.LintToolchainDigest(base)
	base.Command[0] = "/usr/local/bin/other-linter"
	if second := audit.LintToolchainDigest(base); first == second {
		t.Fatal("digest did not change when external executable changed")
	}
	base.Command[0] = "/usr/local/bin/glint"
	base.Command[1] = "--different-argument"
	if second := audit.LintToolchainDigest(base); first == second {
		t.Fatal("digest did not change when external argument changed")
	}
}

func TestExternalLintProviderUsesReadOnlySourceAndSeparateWritableMounts(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest}
	provider := newExternalLintProviderForTest(t, docker)
	docker.run = func(_ context.Context, runRequest audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, runRequest, "passed")
		return nil
	}
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	runs := docker.RunRequests()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0]
	if got, want := run.Image, "registry.example.com/team/glint@"+testImageDigest; got != want {
		t.Fatalf("image = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(run.Args, []string{"/usr/local/bin/glint", "--report-json", "/output/report.json"}) {
		t.Fatalf("command was not kept as an argument array: %#v", run.Args)
	}
	wantMounts := []audit.ContainerMount{{Source: request.ProjectRoot, Target: "/source", ReadOnly: true}, {Source: request.CacheRoot, Target: "/cache"}, {Source: request.RunDir, Target: "/output"}}
	if !reflect.DeepEqual(run.Mounts, wantMounts) {
		t.Fatalf("mounts = %#v, want %#v", run.Mounts, wantMounts)
	}
	if docker.pulls[0] != "registry.example.com/team/glint:v3" {
		t.Fatalf("pulls = %#v", docker.pulls)
	}
}

func TestExternalLintProviderRemovesCompletedContainerWithoutAutoRemove(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest}
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, run, "passed")
		return nil
	}
	provider := newExternalLintProviderForTest(t, docker)
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	runs := docker.RunRequests()
	if len(runs) != 1 || runs[0].AutoRemove {
		t.Fatalf("external run auto-remove = %#v", runs)
	}
	docker.mu.Lock()
	removes := append([]string(nil), docker.removes...)
	docker.mu.Unlock()
	if len(removes) != 1 || removes[0] != runs[0].Name {
		t.Fatalf("completed container removes = %#v", removes)
	}
}

func TestExternalLintProviderRejectsMutableOrMismatchedIdentity(t *testing.T) {
	request := lintRequestForTest(t)
	for name, docker := range map[string]*fakeLintDocker{"mutable inspect": {digest: ""}, "mismatched report": {digest: testImageDigest}} {
		t.Run(name, func(t *testing.T) {
			provider := newExternalLintProviderForTest(t, docker)
			docker.run = func(_ context.Context, runRequest audit.ContainerRunRequest, _ io.Writer) error {
				writeExternalLintReportForTest(t, request, runRequest, "passed")
				if name == "mismatched report" {
					reportPath := filepath.Join(request.RunDir, "report.json")
					content, err := os.ReadFile(reportPath)
					if err != nil {
						t.Fatal(err)
					}
					content = []byte(strings.Replace(string(content), `"provider":"team-ci"`, `"provider":"other"`, 1))
					if err := os.WriteFile(reportPath, content, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			}
			if _, err := provider.Run(context.Background(), request); err == nil {
				t.Fatal("invalid image or report identity was accepted")
			}
		})
	}
}

func TestExternalLintProviderCancellationStopsAndRemovesContainer(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest, started: make(chan struct{})}
	docker.run = func(ctx context.Context, _ audit.ContainerRunRequest, _ io.Writer) error {
		close(docker.started)
		<-ctx.Done()
		return ctx.Err()
	}
	provider := newExternalLintProviderForTest(t, docker)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := provider.Run(ctx, request); done <- err }()
	<-docker.started
	cancel()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("external provider did not return after cancellation")
	}
	if len(docker.stops) != 1 || len(docker.removes) != 1 {
		t.Fatalf("cleanup stop/remove = %#v/%#v", docker.stops, docker.removes)
	}
}

func TestExternalLintProviderCancellationIsBoundedWhenRunAndStopHang(t *testing.T) {
	restore := audit.SetExternalLintCleanupTimeoutForTest(30 * time.Millisecond)
	t.Cleanup(restore)
	request := lintRequestForTest(t)
	runRelease := make(chan struct{})
	stopRelease := make(chan struct{})
	docker := &fakeLintDocker{digest: testImageDigest, started: make(chan struct{})}
	docker.run = func(_ context.Context, _ audit.ContainerRunRequest, _ io.Writer) error {
		close(docker.started)
		<-runRelease
		return nil
	}
	docker.stop = func(context.Context, string, time.Duration) error {
		<-stopRelease
		return errors.New("raw stop failure")
	}
	docker.remove = func(context.Context, string) error { return errors.New("raw remove failure") }
	provider := newExternalLintProviderForTest(t, docker)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := provider.Run(ctx, request); done <- err }()
	<-docker.started
	started := time.Now()
	cancel()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "stop external lint container") || !strings.Contains(err.Error(), "remove external lint container") {
			t.Fatalf("cancellation error = %v", err)
		}
		if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
			t.Fatalf("cancellation took %s", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("external provider waited for hung Docker run or stop")
	}
	close(stopRelease)
	close(runRelease)
}

func TestExternalLintProviderCancellationPreservesDeadlineCause(t *testing.T) {
	restore := audit.SetExternalLintCleanupTimeoutForTest(20 * time.Millisecond)
	t.Cleanup(restore)
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest, started: make(chan struct{})}
	release := make(chan struct{})
	docker.run = func(_ context.Context, _ audit.ContainerRunRequest, _ io.Writer) error {
		close(docker.started)
		<-release
		return nil
	}
	provider := newExternalLintProviderForTest(t, docker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := provider.Run(ctx, request)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation cause = %v", err)
	}
}

func TestExternalLintProviderCancellationPreservesCustomParentCause(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest, started: make(chan struct{})}
	release := make(chan struct{})
	docker.run = func(_ context.Context, _ audit.ContainerRunRequest, _ io.Writer) error {
		close(docker.started)
		<-release
		return nil
	}
	provider := newExternalLintProviderForTest(t, docker)
	cause := errors.New("lint run superseded")
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan error, 1)
	go func() { _, err := provider.Run(ctx, request); done <- err }()
	<-docker.started
	cancel(cause)
	err := <-done
	close(release)
	if !errors.Is(err, cause) {
		t.Fatalf("cancellation cause = %v, want custom cause", err)
	}
}

func TestExternalLintProviderPreservesCancellationBeforeContainerExecution(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		cancelAt       string
		operationError bool
	}{
		{name: "pre-cancelled context", cancelAt: "before pull"},
		{name: "pull returns error after cancellation", cancelAt: "pull", operationError: true},
		{name: "pull ignores cancellation", cancelAt: "pull"},
		{name: "inspect returns error after cancellation", cancelAt: "inspect", operationError: true},
		{name: "inspect ignores cancellation", cancelAt: "inspect"},
		{name: "immediately before run", cancelAt: "before run"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := lintRequestForTest(t)
			docker := &fakeLintDocker{digest: testImageDigest}
			provider := newExternalLintProviderForTest(t, docker)
			cause := errors.New("audit session superseded")
			operationErr := errors.New("docker operation stopped")
			ctx, cancel := context.WithCancelCause(context.Background())
			t.Cleanup(func() { cancel(nil) })
			switch testCase.cancelAt {
			case "before pull":
				cancel(cause)
			case "pull":
				docker.pull = func(context.Context, string) error {
					cancel(cause)
					if testCase.operationError {
						return operationErr
					}
					return nil
				}
			case "inspect":
				docker.inspect = func(context.Context, string) (audit.ImageInspection, error) {
					cancel(cause)
					if testCase.operationError {
						return audit.ImageInspection{}, operationErr
					}
					return audit.ImageInspection{RepoDigests: []string{"registry.example.com/team/glint@" + testImageDigest}}, nil
				}
			case "before run":
				restore := provider.SetExternalLintLifecycleHooksForTest(func() { cancel(cause) }, nil, nil)
				t.Cleanup(restore)
			}
			report, err := provider.Run(ctx, request)
			if report.Status != string(audit.StatusCancelled) || !errors.Is(err, cause) {
				t.Fatalf("report/error = %#v/%v", report, err)
			}
			if testCase.operationError && !errors.Is(err, operationErr) {
				t.Fatalf("operation error was not retained: %v", err)
			}
			docker.mu.Lock()
			pulls, runs, stops, removes := len(docker.pulls), len(docker.runs), len(docker.stops), len(docker.removes)
			docker.mu.Unlock()
			if testCase.cancelAt == "before pull" && pulls != 0 {
				t.Fatalf("pulls = %d, want no Docker calls for pre-cancelled context", pulls)
			}
			if runs != 0 || stops != 0 || removes != 0 {
				t.Fatalf("container calls = run:%d stop:%d remove:%d, want none", runs, stops, removes)
			}
		})
	}
}

func TestExternalLintProviderCleansExactlyOnceAcrossRunCancellationBoundary(t *testing.T) {
	t.Run("cancellation before outer check stops then removes once", func(t *testing.T) {
		request := lintRequestForTest(t)
		docker := &fakeLintDocker{digest: testImageDigest}
		docker.run = func(_ context.Context, _ audit.ContainerRunRequest, _ io.Writer) error { return nil }
		provider := newExternalLintProviderForTest(t, docker)
		afterRun := make(chan struct{})
		releaseAfterRun := make(chan struct{})
		restore := provider.SetExternalLintLifecycleHooksForTest(nil, func() {
			close(afterRun)
			<-releaseAfterRun
		}, nil)
		t.Cleanup(restore)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, err := provider.Run(ctx, request); done <- err }()
		<-afterRun
		cancel()
		close(releaseAfterRun)
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
		docker.mu.Lock()
		stops, removes := len(docker.stops), len(docker.removes)
		docker.mu.Unlock()
		if stops != 1 || removes != 1 {
			t.Fatalf("cleanup calls = stop:%d remove:%d, want exactly one each", stops, removes)
		}
	})

	t.Run("cancellation after outer check removes completed container once", func(t *testing.T) {
		request := lintRequestForTest(t)
		docker := &fakeLintDocker{digest: testImageDigest}
		docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
			writeExternalLintReportForTest(t, request, run, "passed")
			return nil
		}
		provider := newExternalLintProviderForTest(t, docker)
		afterCheck := make(chan struct{})
		releaseAfterCheck := make(chan struct{})
		restore := provider.SetExternalLintLifecycleHooksForTest(nil, nil, func() {
			close(afterCheck)
			<-releaseAfterCheck
		})
		t.Cleanup(restore)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct {
			report audit.LintReport
			err    error
		}, 1)
		go func() {
			report, err := provider.Run(ctx, request)
			done <- struct {
				report audit.LintReport
				err    error
			}{report: report, err: err}
		}()
		<-afterCheck
		cancel()
		close(releaseAfterCheck)
		result := <-done
		if result.err != nil || result.report.Status != "passed" {
			t.Fatalf("report/error = %#v/%v", result.report, result.err)
		}
		docker.mu.Lock()
		stops, removes := len(docker.stops), len(docker.removes)
		docker.mu.Unlock()
		if stops != 0 || removes != 1 {
			t.Fatalf("cleanup calls = stop:%d remove:%d, want completed remove only", stops, removes)
		}
	})
}

func TestExternalLintProviderLifecycleHooksAreProviderScoped(t *testing.T) {
	firstRequest := lintRequestForTest(t)
	secondRequest := lintRequestForTest(t)
	firstDocker := &fakeLintDocker{digest: testImageDigest}
	firstDocker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, firstRequest, run, "passed")
		return nil
	}
	secondDocker := &fakeLintDocker{digest: testImageDigest}
	secondDocker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, secondRequest, run, "passed")
		return nil
	}
	firstProvider := newExternalLintProviderForTest(t, firstDocker)
	secondProvider := newExternalLintProviderForTest(t, secondDocker)
	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})
	restore := firstProvider.SetExternalLintLifecycleHooksForTest(nil, func() {
		close(hookEntered)
		<-releaseHook
	}, nil)
	t.Cleanup(restore)
	firstDone := make(chan error, 1)
	go func() { _, err := firstProvider.Run(context.Background(), firstRequest); firstDone <- err }()
	<-hookEntered
	secondDone := make(chan error, 1)
	go func() { _, err := secondProvider.Run(context.Background(), secondRequest); secondDone <- err }()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("hook on first provider blocked second provider")
	}
	close(releaseHook)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestExternalLintProviderKeepsValidReportsWhenCompletedRemoveFails(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		exitCode int
		status   string
	}{
		{name: "passed", exitCode: 0, status: "passed"},
		{name: "findings", exitCode: 1, status: "failed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := lintRequestForTest(t)
			docker := &fakeLintDocker{digest: testImageDigest}
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeExternalLintReportForTest(t, request, run, testCase.status)
				return commandExitErrorForTest(t, testCase.exitCode)
			}
			docker.remove = func(context.Context, string) error { return errors.New("raw remove failure") }
			provider := newExternalLintProviderForTest(t, docker)
			report, err := provider.Run(context.Background(), request)
			if err != nil || report.Status != testCase.status {
				t.Fatalf("report/error = %#v/%v", report, err)
			}
			log, err := os.ReadFile(filepath.Join(request.RunDir, "external-lint.log"))
			if err != nil || !strings.Contains(string(log), "remove completed external lint container") {
				t.Fatalf("cleanup log/error = %q/%v", log, err)
			}
		})
	}
}

func TestExternalLintProviderJoinsCleanupDiagnosticsForInfrastructureExit(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{digest: testImageDigest}
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, run, "failed")
		return commandExitErrorForTest(t, 125)
	}
	docker.remove = func(context.Context, string) error { return errors.New("raw remove failure") }
	provider := newExternalLintProviderForTest(t, docker)
	_, err := provider.Run(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "infrastructure exit code 125") || !strings.Contains(err.Error(), "remove completed external lint container") {
		t.Fatalf("infrastructure cleanup error = %v", err)
	}
}

func TestExternalLintProviderUsesDistinctStableContainerNames(t *testing.T) {
	docker := &fakeLintDocker{digest: testImageDigest}
	provider := newExternalLintProviderForTest(t, docker)
	first := lintRequestForTest(t)
	second := lintRequestForTest(t)
	second.ProjectID = "project-other"
	second.SessionID = "session-other"
	second.RunID = first.RunID
	second.TargetID = "target-other"
	requestsByRunDir := map[string]audit.LintRequest{first.RunDir: first, second.RunDir: second}
	docker.run = func(_ context.Context, request audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, requestsByRunDir[request.Mounts[2].Source], request, "passed")
		return nil
	}
	var group sync.WaitGroup
	errorsByRun := make(chan error, 2)
	for _, request := range []audit.LintRequest{first, second} {
		group.Add(1)
		go func(request audit.LintRequest) {
			defer group.Done()
			_, err := provider.Run(context.Background(), request)
			errorsByRun <- err
		}(request)
	}
	group.Wait()
	close(errorsByRun)
	for err := range errorsByRun {
		if err != nil {
			t.Fatal(err)
		}
	}
	runs := docker.RunRequests()
	if len(runs) != 2 || runs[0].Name == runs[1].Name {
		t.Fatalf("container names = %#v", runs)
	}
	for _, run := range runs {
		if len(run.Name) > 63 || !strings.HasPrefix(run.Name, "govard-audit-") || !strings.HasSuffix(run.Name, "-lint") {
			t.Fatalf("unsafe container name %q", run.Name)
		}
	}
}

func TestExternalLintProviderSelectsMatchingRepoDigest(t *testing.T) {
	request := lintRequestForTest(t)
	docker := &fakeLintDocker{repoDigests: []string{
		"registry.other.test/team/glint@" + testImageDigest,
		"registry.example.com/team/glint@" + testImageDigest,
	}}
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, run, "passed")
		return nil
	}
	provider := newExternalLintProviderForTest(t, docker)
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	docker = &fakeLintDocker{repoDigests: []string{"localhost:5000/team/glint@" + testImageDigest}}
	provider = newExternalLintProviderForTest(t, docker)
	if _, err := provider.Run(context.Background(), request); err == nil {
		t.Fatal("digest from a different repository was accepted")
	}
	portDocker := &fakeLintDocker{repoDigests: []string{"localhost:5000/team/glint@" + testImageDigest}}
	portProvider, err := audit.NewExternalLintProvider(audit.ExternalLintOptions{ID: "team-ci", Config: engine.ExternalLintProviderConfig{Type: "docker", Image: "localhost:5000/team/glint:v3", Command: []string{"/tool", "--report-json", "/output/report.json"}}, Docker: portDocker})
	if err != nil {
		t.Fatal(err)
	}
	portDocker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, run, "passed")
		return nil
	}
	if _, err := portProvider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func TestExternalLintProviderCopiesConfiguredCommand(t *testing.T) {
	request := lintRequestForTest(t)
	command := []string{"/usr/local/bin/glint", "--report-json", "/output/report.json"}
	docker := &fakeLintDocker{digest: testImageDigest}
	provider, err := audit.NewExternalLintProvider(audit.ExternalLintOptions{ID: "team-ci", Config: engine.ExternalLintProviderConfig{Type: "docker", Image: "registry.example.com/team/glint:v3", Command: command}, Docker: docker})
	if err != nil {
		t.Fatal(err)
	}
	command[0] = "/mutated"
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeExternalLintReportForTest(t, request, run, "passed")
		return nil
	}
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := docker.RunRequests()[0].Args[0]; got != "/usr/local/bin/glint" {
		t.Fatalf("provider command mutated to %q", got)
	}
}

func TestExternalLintProviderInterpretsLintExitCodes(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		exitCode  int
		status    string
		wantError bool
	}{
		{name: "passed", exitCode: 0, status: "passed"},
		{name: "findings", exitCode: 1, status: "failed"},
		{name: "unexpected", exitCode: 2, status: "failed", wantError: true},
		{name: "docker infrastructure", exitCode: 125, status: "failed", wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := lintRequestForTest(t)
			docker := &fakeLintDocker{digest: testImageDigest}
			docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
				writeExternalLintReportForTest(t, request, run, testCase.status)
				return commandExitErrorForTest(t, testCase.exitCode)
			}
			provider := newExternalLintProviderForTest(t, docker)
			report, err := provider.Run(context.Background(), request)
			if testCase.wantError {
				if err == nil {
					t.Fatalf("exit %d was accepted", testCase.exitCode)
				}
				return
			}
			if err != nil || report.Status != testCase.status {
				t.Fatalf("report/error = %#v/%v", report, err)
			}
		})
	}
}

func TestExecDockerClientUsesArgumentArrays(t *testing.T) {
	var binary string
	var args []string
	client := audit.NewExecDockerClient(func(_ context.Context, gotBinary string, gotArgs []string, _ io.Writer, _ io.Writer) error {
		binary = gotBinary
		args = append([]string(nil), gotArgs...)
		return nil
	})
	err := client.Run(context.Background(), audit.ContainerRunRequest{Name: "audit-run", Image: "registry.example.test/linter@" + testImageDigest, Args: []string{"/tool", "argument with spaces", ";not-a-shell-command"}, Mounts: []audit.ContainerMount{{Source: "/project", Target: "/source", ReadOnly: true}}, AutoRemove: true}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if binary != "docker" {
		t.Fatalf("binary = %q", binary)
	}
	if !containsAdjacent(args, []string{"run", "--name", "audit-run", "--rm", "-v", "/project:/source:ro", "registry.example.test/linter@" + testImageDigest, "/tool", "argument with spaces", ";not-a-shell-command"}) {
		t.Fatalf("docker argument array = %#v", args)
	}
	if containsAdjacent(args, []string{"--user"}) {
		t.Fatalf("docker argument array imposed a user without one being requested: %#v", args)
	}
}

func TestExecDockerClientPassesRequestedHostUser(t *testing.T) {
	var args []string
	client := audit.NewExecDockerClient(func(_ context.Context, _ string, gotArgs []string, _ io.Writer, _ io.Writer) error {
		args = append([]string(nil), gotArgs...)
		return nil
	})
	err := client.Run(context.Background(), audit.ContainerRunRequest{Name: "audit-run", User: "1000:1000", Image: "registry.example.test/linter@" + testImageDigest, Args: []string{"/tool"}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacent(args, []string{"run", "--name", "audit-run", "--user", "1000:1000"}) {
		t.Fatalf("docker argument array = %#v, want the host user right after the container name", args)
	}
}

func TestValidateLintReportRejectsMismatchedIdentity(t *testing.T) {
	request := lintRequestForTest(t)
	valid := completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")
	for name, mutate := range map[string]func(*audit.LintReport){
		"provider":    func(report *audit.LintReport) { report.Provider = "other" },
		"session":     func(report *audit.LintReport) { report.SessionID = "other" },
		"run":         func(report *audit.LintReport) { report.RunID = "other" },
		"project":     func(report *audit.LintReport) { report.ProjectID = "other" },
		"target ID":   func(report *audit.LintReport) { report.TargetID = "other" },
		"target mode": func(report *audit.LintReport) { report.TargetMode = types.AuditTargetStandalone },
		"target path": func(report *audit.LintReport) { report.TargetPath = "/other/path" },
	} {
		t.Run(name, func(t *testing.T) {
			report := valid
			mutate(&report)
			if err := audit.ValidateLintReport(request, report); err == nil {
				t.Fatal("mismatched report identity was accepted")
			}
		})
	}
}

func TestValidateLintReportRejectsInvalidPHPMatrix(t *testing.T) {
	request := lintRequestForTest(t)
	valid := completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")
	for name, mutate := range map[string]func(*audit.LintReport){
		"missing image digest":     func(report *audit.LintReport) { report.ImageDigest = "" },
		"missing toolchain digest": func(report *audit.LintReport) { report.ToolchainDigest = "" },
		"missing result":           func(report *audit.LintReport) { report.PHPResults = report.PHPResults[1:] },
		"duplicate result":         func(report *audit.LintReport) { report.PHPResults[1].PHPVersion = report.PHPResults[0].PHPVersion },
		"unsupported PHP":          func(report *audit.LintReport) { report.PHPResults[0].PHPVersion = "9.9" },
		"missing phase":            func(report *audit.LintReport) { report.PHPResults[0].Phases = nil },
		"invalid outcome":          func(report *audit.LintReport) { report.PHPResults[0].Outcome = "running" },
	} {
		t.Run(name, func(t *testing.T) {
			report := cloneLintReportForTest(valid)
			mutate(&report)
			if err := audit.ValidateLintReport(request, report); err == nil {
				t.Fatal("invalid PHP matrix was accepted")
			}
		})
	}
}

func TestValidateLintReportAcceptsConsistentAggregateStatuses(t *testing.T) {
	request := lintRequestForTest(t)
	for _, testCase := range []struct {
		name     string
		outcomes []string
		status   string
	}{
		{name: "passed", outcomes: []string{"passed", "passed"}, status: "passed"},
		{name: "passed with unsupported", outcomes: []string{"passed", "unsupported"}, status: "passed"},
		{name: "unsupported", outcomes: []string{"unsupported", "unsupported"}, status: "unsupported"},
		{name: "failed", outcomes: []string{"passed", "failed"}, status: "failed"},
		{name: "infra error", outcomes: []string{"passed", "infra_error"}, status: "infra_error"},
		{name: "cancelled", outcomes: []string{"passed", "cancelled"}, status: "cancelled"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			report := completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", testCase.status)
			for index, outcome := range testCase.outcomes {
				report.PHPResults[index].Outcome = outcome
				report.PHPResults[index].Phases[0].Status = outcome
			}
			if err := audit.ValidateLintReport(request, report); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateLintReportRejectsSelectedMatrixOrCompletenessMismatch(t *testing.T) {
	request := lintRequestForTest(t)
	valid := completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")
	for name, mutate := range map[string]func(*audit.LintReport){
		"selected versions":  func(report *audit.LintReport) { report.SelectedPHPVersions = []string{"8.1"} },
		"matrix complete":    func(report *audit.LintReport) { report.MatrixComplete = false },
		"aggregate mismatch": func(report *audit.LintReport) { report.Status = "failed" },
	} {
		t.Run(name, func(t *testing.T) {
			report := cloneLintReportForTest(valid)
			mutate(&report)
			if err := audit.ValidateLintReport(request, report); err == nil {
				t.Fatal("inconsistent report was accepted")
			}
		})
	}
}

func TestValidateLintReportEnforcesTargetModeCompleteness(t *testing.T) {
	request := lintRequestForTest(t)
	request.Target.Mode = types.AuditTargetStandalone
	request.Profile.StandalonePHPVersions = []string{"8.1", "8.2"}
	request.SelectedPHPVersions = []string{"8.1"}
	request.MatrixComplete = false
	if err := audit.ValidateLintReport(request, completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")); err != nil {
		t.Fatal(err)
	}
	request.MatrixComplete = true
	if err := audit.ValidateLintReport(request, completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")); err == nil {
		t.Fatal("partial standalone matrix was marked complete")
	}
	request.Target.Mode = types.AuditTargetProject
	request.MatrixComplete = false
	if err := audit.ValidateLintReport(request, completeLintReportForTest(request, testImageDigest, "sha256:bbbbbbbb", "passed")); err == nil {
		t.Fatal("project matrix was allowed to be partial")
	}
}

const testImageDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeLintDocker struct {
	mu          sync.Mutex
	digest      string
	repoDigests []string
	pulls       []string
	runs        []audit.ContainerRunRequest
	stops       []string
	removes     []string
	pull        func(context.Context, string) error
	inspect     func(context.Context, string) (audit.ImageInspection, error)
	run         func(context.Context, audit.ContainerRunRequest, io.Writer) error
	stop        func(context.Context, string, time.Duration) error
	remove      func(context.Context, string) error
	started     chan struct{}
}

func (docker *fakeLintDocker) Pull(ctx context.Context, image string) error {
	docker.mu.Lock()
	docker.pulls = append(docker.pulls, image)
	pull := docker.pull
	docker.mu.Unlock()
	if pull != nil {
		return pull(ctx, image)
	}
	return nil
}
func (docker *fakeLintDocker) Inspect(ctx context.Context, image string) (audit.ImageInspection, error) {
	docker.mu.Lock()
	inspect := docker.inspect
	repoDigests := append([]string(nil), docker.repoDigests...)
	digest := docker.digest
	docker.mu.Unlock()
	if inspect != nil {
		return inspect(ctx, image)
	}
	if len(repoDigests) > 0 {
		return audit.ImageInspection{RepoDigests: repoDigests}, nil
	}
	if digest == "" {
		return audit.ImageInspection{}, nil
	}
	return audit.ImageInspection{RepoDigests: []string{"registry.example.com/team/glint@" + digest}}, nil
}
func (docker *fakeLintDocker) Build(context.Context, string, string, map[string]string) error {
	return nil
}
func (docker *fakeLintDocker) Run(ctx context.Context, request audit.ContainerRunRequest, output io.Writer) error {
	docker.mu.Lock()
	docker.runs = append(docker.runs, request)
	run := docker.run
	docker.mu.Unlock()
	if run == nil {
		return nil
	}
	return run(ctx, request, output)
}
func (docker *fakeLintDocker) Stop(ctx context.Context, name string, timeout time.Duration) error {
	docker.mu.Lock()
	docker.stops = append(docker.stops, name)
	stop := docker.stop
	docker.mu.Unlock()
	if stop != nil {
		return stop(ctx, name, timeout)
	}
	return nil
}
func (docker *fakeLintDocker) Remove(ctx context.Context, name string) error {
	docker.mu.Lock()
	docker.removes = append(docker.removes, name)
	remove := docker.remove
	docker.mu.Unlock()
	if remove != nil {
		return remove(ctx, name)
	}
	return nil
}

func (docker *fakeLintDocker) RunRequests() []audit.ContainerRunRequest {
	docker.mu.Lock()
	defer docker.mu.Unlock()
	return append([]audit.ContainerRunRequest(nil), docker.runs...)
}

func newExternalLintProviderForTest(t *testing.T, docker audit.DockerClient) *audit.ExternalLintProvider {
	t.Helper()
	provider, err := audit.NewExternalLintProvider(audit.ExternalLintOptions{ID: "team-ci", Config: engine.ExternalLintProviderConfig{Type: "docker", Image: "registry.example.com/team/glint:v3", Command: []string{"/usr/local/bin/glint", "--report-json", "/output/report.json"}}, Docker: docker})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func lintRequestForTest(t *testing.T) audit.LintRequest {
	t.Helper()
	root := t.TempDir()
	return audit.LintRequest{ProjectRoot: root, ProjectID: "project-aabbccdd", SessionID: "session-aabbccdd", RunID: "run-0001", Provider: "team-ci", TargetID: "target-aabbccdd", Target: types.AuditTarget{Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject}, RunDir: filepath.Join(root, "run-0001"), CacheRoot: filepath.Join(root, "cache", "lint"), Scope: audit.ScopeProject, Jobs: 2, Profile: types.AuditLintProfile{ProjectPHPVersions: []string{"8.1", "8.2"}, Linters: []string{"phpcs", "phpstan"}, PHPStanLevel: 5}, SelectedPHPVersions: []string{"8.1", "8.2"}, MatrixComplete: true}
}

func writeExternalLintReportForTest(t *testing.T, request audit.LintRequest, run audit.ContainerRunRequest, status string) {
	t.Helper()
	report := completeLintReportForTest(request, run.Environment["GOVARD_LINT_IMAGE_DIGEST"], run.Environment["GOVARD_LINT_TOOLCHAIN_DIGEST"], status)
	content, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.RunDir, "report.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func completeLintReportForTest(request audit.LintRequest, imageDigest, toolchainDigest, status string) audit.LintReport {
	report := audit.LintReport{SchemaVersion: audit.LintReportSchemaVersion, Provider: request.Provider, SessionID: request.SessionID, RunID: request.RunID, ProjectID: request.ProjectID, TargetID: request.TargetID, TargetMode: request.Target.Mode, TargetPath: request.Target.TargetPath, ImageDigest: imageDigest, ToolchainDigest: toolchainDigest, Status: status, DurationMS: 1, SelectedPHPVersions: request.PHPVersions(), MatrixComplete: request.MatrixComplete}
	for _, version := range request.SelectedPHPVersions {
		report.PHPResults = append(report.PHPResults, audit.LintPHPResult{PHPVersion: version, Outcome: status, DurationMS: 1, Phases: []audit.LintPhase{{Name: "phpcs", Status: status, DurationMS: 1}}})
	}
	return report
}

func cloneLintReportForTest(report audit.LintReport) audit.LintReport {
	cloned := report
	cloned.PHPResults = append([]audit.LintPHPResult(nil), report.PHPResults...)
	for index := range cloned.PHPResults {
		cloned.PHPResults[index].Phases = append([]audit.LintPhase(nil), report.PHPResults[index].Phases...)
	}
	return cloned
}

func containsAdjacent(values, fragment []string) bool {
	for index := 0; index+len(fragment) <= len(values); index++ {
		if reflect.DeepEqual(values[index:index+len(fragment)], fragment) {
			return true
		}
	}
	return false
}

func commandExitErrorForTest(t *testing.T, code int) error {
	t.Helper()
	if code == 0 {
		return nil
	}
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
}
