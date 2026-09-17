package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"

	"govard/internal/engine"
	"govard/internal/runtime"
)

// SandboxLiveness reports what state, if any, a project's sandbox container
// is in, as seen by ResolveSyntheticSandboxRemote.
type SandboxLiveness string

const (
	// SandboxLivenessAbsent means no container by the deterministic sandbox
	// name exists at all: never created, or removed by `sandbox down --purge`.
	SandboxLivenessAbsent SandboxLiveness = "absent"
	// SandboxLivenessDormant means the container exists but is stopped
	// (plain `sandbox down` without --purge stops it and keeps .govard/sandbox/).
	SandboxLivenessDormant SandboxLiveness = "dormant"
	// SandboxLivenessRunning means the container is up and a RemoteConfig
	// was built.
	SandboxLivenessRunning SandboxLiveness = "running"
)

// ResolveSyntheticSandboxRemote resolves "sandbox" as a remote from live
// Docker state instead of from .govard.yml/.govard.local.yml. It reads no
// config file and writes nothing: only `sandbox up` may create a container.
//
// On any liveness other than running, the returned error already carries the
// exact user-facing message; callers propagate it through their own existing
// error path without reformatting it.
func ResolveSyntheticSandboxRemote(ctx context.Context, projectName string) (engine.RemoteConfig, SandboxLiveness, error) {
	root, err := os.Getwd()
	if err != nil {
		return engine.RemoteConfig{}, SandboxLivenessAbsent, fmt.Errorf("resolve the project directory: %w", err)
	}
	return resolveSyntheticSandboxRemote(ctx, NewDockerCLI(), LocalRunner{}, root, projectName)
}

// ResolveSyntheticSandboxRemoteForTest exposes the injectable core to the
// tests/ package, so a unit test never shells out to a real docker/git binary.
func ResolveSyntheticSandboxRemoteForTest(ctx context.Context, runtime SandboxRuntime, git Runner, projectRoot, projectName string) (engine.RemoteConfig, SandboxLiveness, error) {
	return resolveSyntheticSandboxRemote(ctx, runtime, git, projectRoot, projectName)
}

// resolveSyntheticSandboxRemoteFn is the seam resolveBaseOptions/HostForConfig/
// LoadSandboxRemote call through. Production always calls the real
// ResolveSyntheticSandboxRemote; a test replaces it for the duration of one
// test with StubResolveSyntheticSandboxRemoteForTest.
var resolveSyntheticSandboxRemoteFn = ResolveSyntheticSandboxRemote

// StubResolveSyntheticSandboxRemoteForTest replaces the production sandbox
// resolution entry point for the duration of a test and returns a func that
// restores the real one.
func StubResolveSyntheticSandboxRemoteForTest(fn func(ctx context.Context, projectName string) (engine.RemoteConfig, SandboxLiveness, error)) func() {
	original := resolveSyntheticSandboxRemoteFn
	resolveSyntheticSandboxRemoteFn = fn
	return func() { resolveSyntheticSandboxRemoteFn = original }
}

func resolveSyntheticSandboxRemote(ctx context.Context, sandboxRuntime SandboxRuntime, git Runner, projectRoot, projectName string) (engine.RemoteConfig, SandboxLiveness, error) {
	if err := requireSandboxCapableRuntime(); err != nil {
		return engine.RemoteConfig{}, SandboxLivenessAbsent, err
	}

	container := SandboxContainerName(projectName, projectRoot)
	exists, err := sandboxRuntime.ContainerExists(ctx, container)
	if err != nil {
		return engine.RemoteConfig{}, SandboxLivenessAbsent, fmt.Errorf("check the sandbox container: %w", err)
	}
	if !exists {
		return engine.RemoteConfig{}, SandboxLivenessAbsent, fmt.Errorf(
			"unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it")
	}

	running, err := sandboxRuntime.ContainerRunning(ctx, container)
	if err != nil {
		return engine.RemoteConfig{}, SandboxLivenessDormant, fmt.Errorf("check the sandbox container: %w", err)
	}
	if !running {
		return engine.RemoteConfig{}, SandboxLivenessDormant, fmt.Errorf(
			"sandbox container %s is not running; run 'govard sandbox up' to start it", container)
	}

	profile := containerLabelOrEmpty(ctx, sandboxRuntime, container, sandboxProfileLabel)
	php := containerLabelOrEmpty(ctx, sandboxRuntime, container, sandboxPHPLabel)

	port, err := sandboxRuntime.PublishedPort(ctx, container, SandboxSSHPort)
	if err != nil {
		return engine.RemoteConfig{}, SandboxLivenessRunning, fmt.Errorf("read the sandbox's published SSH port: %w", err)
	}
	webPort := 0
	if SandboxServesWeb(profile) {
		if published, err := sandboxRuntime.PublishedPort(ctx, container, sandboxWebPort); err == nil {
			webPort = published
		}
	}

	remote := SandboxRemoteConfig(profile, php, port, webPort, SandboxDefaultPaths(), SandboxKeyPair{})
	remote.Deploy.Branch = localBranch(ctx, git, projectRoot)
	return remote, SandboxLivenessRunning, nil
}

// requireSandboxCapableRuntime fails fast and explicitly when Docker is
// unavailable, mirroring internal/cmd's requireContainerRuntime (used by
// `audit run`): the same *runtime.MissingError shape the root capability
// gate itself would produce, so a Docker-free command (remote exec, sync)
// asked to resolve "sandbox" on a Docker-less host fails at exit 3 with the
// CAPABILITY_MISSING envelope instead of a raw docker error surfacing deep
// inside resolution.
func requireSandboxCapableRuntime() error {
	err := runtime.Probe(runtime.CapDocker)
	if err == nil {
		return nil
	}
	var missing *runtime.MissingError
	detail := err.Error()
	if errors.As(err, &missing) {
		detail = missing.Detail
	}
	return &runtime.MissingError{
		Caps:   []runtime.Capability{runtime.CapDocker},
		Detail: detail,
		Hint:   "resolving the sandbox remote needs Docker; run `govard sandbox up` once it is available",
	}
}
