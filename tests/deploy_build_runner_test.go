package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"
)

// The container is what makes this flag exist: a project whose private-VCS SSH
// key lives inside it has nothing on the host that can fetch its dependencies,
// so an artifact cannot be assembled there at all (the 2026-09-28 rehearsal
// failed exactly this way). `--runner container` is that way out — and it is
// opt-in, because every other project builds fine on the host shell.
func buildRunnerProject() engine.Config {
	return engine.Config{ProjectName: "sample", Framework: "magento2"}
}

// buildRunnerRoot is a project root plus the output directory a build would
// write inside it, which is the shape the container runner can map.
func buildRunnerRoot(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create the project root: %v", err)
	}
	return root, filepath.Join(root, "artifacts")
}

// stubMissingContainerRuntime makes every docker probe report a host with no
// container runtime, so a test that expects a build to work anyway proves it
// never probed.
func stubMissingContainerRuntime(t *testing.T) {
	t.Helper()
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return errors.New("cannot connect to the docker daemon") },
		func(context.Context) error { return errors.New("cannot connect to the docker daemon") },
	)
	t.Cleanup(restore)
}

// stubPresentContainerRuntime reports a host that has a container runtime,
// without contacting the developer's own daemon: the unit suite must not depend
// on what happens to be running.
func stubPresentContainerRuntime(t *testing.T) {
	t.Helper()
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	t.Cleanup(restore)
	t.Cleanup(runtime.StubLookPathForTest(func(string) (string, error) { return "/usr/bin/docker", nil }))
}

// `deploy build` is declared `none` and must stay that way, or the Docker-free
// surface stops being one. The probes here report a host with no container
// runtime at all: a runner resolution that asked for Docker would fail, so
// returning the host shell is proof that it did not.
func TestDeployBuildRunnerDefaultsToTheHostShellWithoutProbing(t *testing.T) {
	stubMissingContainerRuntime(t)
	root, output := buildRunnerRoot(t)

	for _, name := range []string{"", "host"} {
		runner, err := cmd.DeployBuildRunnerForTest(buildRunnerProject(), name, root, output)
		if err != nil {
			t.Fatalf("--runner %q: %v", name, err)
		}
		if _, ok := runner.(deploy.LocalRunner); !ok {
			t.Errorf("--runner %q = %T, want deploy.LocalRunner", name, runner)
		}
	}
}

// A container build runs the recipe's tasks through the project's own app
// container, so the runner has to name that container, the workdir it mounts,
// and the account the exec uses — the three things the host shell cannot guess.
func TestDeployBuildRunnerContainerResolvesTheProjectContainer(t *testing.T) {
	stubPresentContainerRuntime(t)
	root, output := buildRunnerRoot(t)

	runner, err := cmd.DeployBuildRunnerForTest(buildRunnerProject(), "container", root, output)
	if err != nil {
		t.Fatalf("--runner container: %v", err)
	}
	container, ok := runner.(deploy.ContainerRunner)
	if !ok {
		t.Fatalf("--runner container = %T, want deploy.ContainerRunner", runner)
	}
	if container.Container != "sample-php-1" {
		t.Errorf("container = %q, want the project's app container sample-php-1", container.Container)
	}
	if container.ContainerRoot != "/var/www/html" {
		t.Errorf("container root = %q, want the project workdir /var/www/html", container.ContainerRoot)
	}
	if container.LocalRoot != root {
		t.Errorf("local root = %q, want the checkout %q", container.LocalRoot, root)
	}
	if container.User == "" {
		t.Error("user is empty, want the account the project's own exec uses")
	}
}

// The refusal comes before the probe on purpose, and the probes here report a
// host with no container runtime: an implementation that asked for Docker first
// would answer with a missing capability instead of naming the path, and the
// operator would be sent to start a daemon that would not have helped.
func TestDeployBuildRunnerRefusesAnOutputOutsideTheProjectBeforeProbing(t *testing.T) {
	stubMissingContainerRuntime(t)
	root, _ := buildRunnerRoot(t)
	elsewhere := filepath.Join(t.TempDir(), "artifacts")

	runner, err := cmd.DeployBuildRunnerForTest(buildRunnerProject(), "container", root, elsewhere)
	if err == nil {
		t.Fatal("want a refusal when --output is outside the project root")
	}
	if runner != nil {
		t.Errorf("runner = %#v, want nil alongside the refusal", runner)
	}
	if !errors.Is(err, deploy.ErrContainerPathUnmappable) {
		t.Fatalf("error = %v, want deploy.ErrContainerPathUnmappable", err)
	}
	var missing *runtime.MissingError
	if errors.As(err, &missing) {
		t.Fatalf("a path that cannot be mapped is a configuration mistake, not a missing capability: %v", err)
	}
}

// Asking for a container the host cannot provide fails the way every other
// capability failure does — exit 3 with a hint that names both ways out.
func TestDeployBuildRunnerContainerNeedsAContainerRuntime(t *testing.T) {
	stubMissingContainerRuntime(t)
	root, output := buildRunnerRoot(t)

	runner, err := cmd.DeployBuildRunnerForTest(buildRunnerProject(), "container", root, output)
	if err == nil {
		t.Fatal("want a missing-capability error on a host with no container runtime")
	}
	if runner != nil {
		t.Errorf("runner = %#v, want nil alongside the failure", runner)
	}
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want *runtime.MissingError", err)
	}
	if missing.ExitCode() != runtime.CodeCapabilityMissing {
		t.Errorf("exit code = %d, want %d", missing.ExitCode(), runtime.CodeCapabilityMissing)
	}
	for _, want := range []string{"govard env up", "--runner host"} {
		if !strings.Contains(missing.Hint, want) {
			t.Errorf("hint %q must name %q", missing.Hint, want)
		}
	}
}

// A runner name nobody implements is a mistake on the command line, so it exits
// 2 and says which names exist — it is not a failed build.
func TestDeployBuildRunnerRejectsAnUnknownRunner(t *testing.T) {
	stubPresentContainerRuntime(t)
	root, output := buildRunnerRoot(t)

	runner, err := cmd.DeployBuildRunnerForTest(buildRunnerProject(), "nope", root, output)
	if err == nil {
		t.Fatal("want an error for an unknown --runner value")
	}
	if runner != nil {
		t.Errorf("runner = %#v, want nil alongside the error", runner)
	}
	if code := cli.Code(err); code != cli.CodeUsage {
		t.Errorf("exit code = %d, want %d (usage)", code, cli.CodeUsage)
	}
	for _, want := range []string{"host", "container"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name the runner it accepts (%q)", err.Error(), want)
		}
	}
}

// The declared requirement is what the whole Docker-free surface rests on:
// `deploy build` assembles an artifact on a CI runner that has no container
// runtime, and the flag is what the operator opts into needing one.
func TestDeployBuildStaysRequirementFreeWithTheHostAsTheDefault(t *testing.T) {
	command := cmd.DeployBuildCommand()
	requires := runtime.Requires(command)
	if len(requires) != 1 || requires[0] != runtime.CapNone {
		t.Fatalf("%s requires %v, want [none]", command.CommandPath(), requires)
	}
	flag := command.Flags().Lookup("runner")
	if flag == nil {
		t.Fatal("deploy build has no --runner flag")
	}
	if flag.DefValue != "host" {
		t.Errorf("--runner default = %q, want host", flag.DefValue)
	}
}
