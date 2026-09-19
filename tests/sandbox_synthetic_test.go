package tests

import (
	"context"
	"errors"
	"testing"

	"govard/internal/deploy"
	"govard/internal/runtime"
)

func TestResolveSyntheticSandboxRemoteReportsAbsentWithNoContainer(t *testing.T) {
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	fake := absentContainerFake()
	_, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(
		context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{},
		t.TempDir(), "sample-project")
	if liveness != deploy.SandboxLivenessAbsent {
		t.Fatalf("liveness = %q, want absent", liveness)
	}
	if err == nil {
		t.Fatal("an absent sandbox must be a resolvable error, not a silent zero value")
	}
}

func TestResolveSyntheticSandboxRemoteReportsDormantWhenStopped(t *testing.T) {
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	fake := sandboxFake()
	fake.answers["inspect --format {{.State.Running}}"] = "false\n" // container exists, ContainerRunning reports false
	_, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(
		context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{},
		t.TempDir(), "sample-project")
	if liveness != deploy.SandboxLivenessDormant {
		t.Fatalf("liveness = %q, want dormant", liveness)
	}
	if err == nil {
		t.Fatal("a dormant sandbox must be a resolvable error naming the container")
	}
}

func TestResolveSyntheticSandboxRemoteBuildsARemoteWhenRunning(t *testing.T) {
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	fake := sandboxFake() // must answer "true" to `inspect -f {{.State.Running}}` and a published SSH port, matching the existing sandboxFake() helper's defaults used throughout tests/deploy_sandbox_test.go
	remote, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(
		context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{},
		t.TempDir(), "sample-project")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if liveness != deploy.SandboxLivenessRunning {
		t.Fatalf("liveness = %q, want running", liveness)
	}
	if remote.Host != "127.0.0.1" || remote.User != deploy.SandboxUser {
		t.Fatalf("remote = %+v, want the sandbox's real host/user", remote)
	}
	if !remote.Sandbox {
		t.Fatal("the synthetic remote must be marked Sandbox: true, same as SandboxRemoteConfig always sets it")
	}
	if remote.Protected == nil || *remote.Protected {
		t.Fatal("a sandbox must never be protected")
	}
}

func TestSyntheticSandboxDormantPreservesProfileAndPHP(t *testing.T) {
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile("full").containerPHP("8.4")
	fake.answers["inspect --format {{.State.Running}}"] = "false\n" // container exists, ContainerRunning reports false
	_, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(
		context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{},
		root, "sample-project")
	if liveness != deploy.SandboxLivenessDormant {
		t.Fatalf("liveness = %q, want dormant (err=%v)", liveness, err)
	}
	state, statusErr := deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if statusErr != nil {
		t.Fatalf("SandboxStatus: %v", statusErr)
	}
	if state.Profile == "" || state.PHP == "" {
		t.Fatalf("dormant status must keep profile+PHP, got profile=%q php=%q", state.Profile, state.PHP)
	}
	if state.Profile != "full" || state.PHP != "8.4" {
		t.Fatalf("dormant status must read profile+PHP from the container labels, got profile=%q php=%q", state.Profile, state.PHP)
	}
}

// A dormant container's PHP label is only a fallback when it names a real
// series: a value the creation could never have written (the --php flag is
// validated first) is ignored rather than reported, mirroring the reuse path.
func TestSyntheticSandboxDormantIgnoresGarbagePHPLabel(t *testing.T) {
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile("full").containerPHP("not-a-series")
	fake.answers["inspect --format {{.State.Running}}"] = "false\n" // container exists, ContainerRunning reports false
	state, err := deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("SandboxStatus: %v", err)
	}
	if state.PHP != "" {
		t.Fatalf("dormant status must ignore a garbage PHP label, got php=%q", state.PHP)
	}
	if state.Profile != "full" {
		t.Fatalf("dormant status must still keep the profile, got profile=%q", state.Profile)
	}
}

func TestResolveSyntheticSandboxRemoteRequiresDockerExplicitly(t *testing.T) {
	restore := runtime.StubProbesForTest(func(context.Context) error {
		return errors.New("Cannot connect to the Docker daemon")
	}, nil)
	defer restore()
	_, _, err := deploy.ResolveSyntheticSandboxRemoteForTest(
		context.Background(), deploy.NewDockerCLIForTest(absentContainerFake().run), deploy.LocalRunner{},
		t.TempDir(), "sample-project")
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want a *runtime.MissingError so the caller gets exit 3, not a raw docker error", err)
	}
	if missing.Caps[0] != runtime.CapDocker {
		t.Fatalf("caps = %v, want [docker]", missing.Caps)
	}
}
