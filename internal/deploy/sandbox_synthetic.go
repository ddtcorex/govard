package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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

// sandboxPinnedDeploySettings are the deploy settings that describe the sandbox
// container rather than the rehearsal: the deployer account the image ships, how
// its files are written, and the PHP binary and series it actually built. A
// block-supplied value for any of them would make the deploy assert something the
// container does not have — `php_version` most of all, since a deploy refuses to
// run when the target's PHP does not match the declared series. Everything else
// under `deploy:` stays overridable, so a rehearsal can still state its publish
// strategy, retention, `writable_permissions` and the composer binary.
var sandboxPinnedDeploySettings = []string{"owner", "writable_mode", "php_bin", "php_version"}

// SandboxRemoteOverlay returns the remote the sandbox actually resolves to: the
// container-derived identity (base) with the project's `remotes.sandbox` block
// layered on top through an explicit, closed allowlist.
//
// The direction is the whole security property. base is what govard's own
// `sandbox up` created and what the operator sees; the block is an allowlist of
// rehearsal *shape* — capabilities, protected, deploy settings. Host, port, user,
// path, url, auth, paths, the database credentials and the two topology flags
// are never taken from it, because a leaked identity does not fail loudly: the
// rehearsal would silently run against a different machine, at a different path,
// with a different key, and report success.
//
// When base carries no deploy block there is no container-derived deploy
// configuration to protect, so the block's is not applied either — an overlay
// that only ever adds a container's own facts has nothing to layer on.
func SandboxRemoteOverlay(base engine.RemoteConfig, remotes engine.RemoteConfigMap) engine.RemoteConfig {
	configured, ok := sandboxBlock(remotes)
	if !ok {
		return base
	}
	overlay := base
	if configured.Capabilities != nil {
		overlay.Capabilities = configured.Capabilities
	}
	if configured.Protected != nil {
		overlay.Protected = configured.Protected
	}
	if base.Deploy != nil && configured.Deploy != nil {
		deployCfg := mergeDeployConfig(*base.Deploy, *configured.Deploy)
		for _, key := range sandboxPinnedDeploySettings {
			if value, pinned := base.Deploy.Settings[key]; pinned {
				deployCfg.Settings[key] = value
			} else {
				delete(deployCfg.Settings, key)
			}
		}
		overlay.Deploy = &deployCfg
	}
	return overlay
}

// ResolveSandboxRemoteForConfig resolves the synthetic sandbox for a caller
// that already knows which project it is looking at, and layers that project's
// `remotes.sandbox` block over the container's identity.
//
// ResolveSyntheticSandboxRemote answers the same question from the working
// directory, which is the right assumption for a CLI command and the wrong one
// for a surface that resolves a project out of a registry — the desktop app,
// whose buttons name a project path the process never chdir'd into. Both go
// through the same resolution, so a sandbox behaves the same however it was
// reached.
func ResolveSandboxRemoteForConfig(ctx context.Context, cfg engine.Config, projectRoot string) (engine.RemoteConfig, SandboxLiveness, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		working, err := os.Getwd()
		if err != nil {
			return engine.RemoteConfig{}, SandboxLivenessAbsent, fmt.Errorf("resolve the project directory: %w", err)
		}
		root = working
	}

	base, liveness, err := resolveSyntheticSandboxRemote(ctx, NewDockerCLI(), LocalRunner{}, root, cfg.ProjectName)
	if err != nil {
		return engine.RemoteConfig{}, liveness, err
	}
	return SandboxRemoteOverlay(base, cfg.Remotes), liveness, nil
}

// sandboxBlock finds the project's `remotes.sandbox` block, matched the way
// `remote list` and ensureRemoteKnown already match the synthetic name:
// case-insensitively, so a hand-edited "Sandbox:" is the same block and not a
// second remote that silently never applies.
//
// The exact spelling is tried first. Go randomises map iteration, so a project
// carrying both `sandbox:` and `Sandbox:` would otherwise get an arbitrary one
// applied per invocation — the same rehearsal configured two different ways on
// two runs of the same command, with no error to explain it.
func sandboxBlock(remotes engine.RemoteConfigMap) (engine.RemoteConfig, bool) {
	if configured, ok := remotes[SandboxRemoteName]; ok {
		return configured, true
	}
	for name, configured := range remotes {
		if strings.EqualFold(strings.TrimSpace(name), SandboxRemoteName) {
			return configured, true
		}
	}
	return engine.RemoteConfig{}, false
}

// sandboxRemoteForConfig resolves the synthetic sandbox and layers the
// project's configured `remotes.sandbox` block over it. Every deploy-side reader
// of the synthetic remote goes through here, so a configured block can change
// the rehearsal's shape but never its identity.
func sandboxRemoteForConfig(cfg engine.Config) (engine.RemoteConfig, SandboxLiveness, error) {
	base, liveness, err := resolveSyntheticSandboxRemoteFn(context.Background(), cfg.ProjectName)
	if err != nil {
		return engine.RemoteConfig{}, liveness, err
	}
	return SandboxRemoteOverlay(base, cfg.Remotes), liveness, nil
}

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
		// The sentinel supplies the class; the sentence is unchanged, so the
		// "run 'govard sandbox up'" remedy every caller already prints survives.
		// The dormant branch below deliberately carries no sentinel: a sandbox
		// that exists and is stopped is a failure of the container's state, not a
		// name missing from the configuration, and the two must not be merged.
		return engine.RemoteConfig{}, SandboxLivenessAbsent, fmt.Errorf(
			"%w: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it", ErrUnknownRemote)
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
