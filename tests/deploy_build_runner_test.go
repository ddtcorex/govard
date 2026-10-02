package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/spf13/pflag"
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
	logFile := buildDockerStub(t, "true", "")

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
	// The container is probed by name before the runner is handed back.
	calls := buildDockerCalls(t, logFile)
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "inspect ") || !strings.Contains(calls[0], "sample-php-1") {
		t.Errorf("docker calls = %q, want one inspect of sample-php-1", calls)
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
	if code := cli.Code(err); code != cli.CodeConfig {
		t.Errorf("exit code = %d, want %d (configuration)", code, cli.CodeConfig)
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

// containerBuildProject is a project `govard deploy build local --runner
// container` can run against end to end: a Laravel recipe (it declares
// build:frontend), a local remote, and an artifact from an earlier build already
// sitting in the output directory. frontendDirs, when non-empty, enables the
// frontend step. The process moves into the project, because a build reads its
// checkout from the working directory.
func containerBuildProject(t *testing.T, frontendDirs string) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	settings := ""
	if frontendDirs != "" {
		settings = "  settings:\n    frontend_dir: " + frontendDirs + "\n"
	}
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: laravel
domain: sample.test
deploy:
`+settings+`remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    local: true
    deploy:
      deploy_path: `+filepath.Join(root, ".deployer")+`
      branch: main
`)
	output := filepath.Join(root, "artifacts")
	writeFile(t, filepath.Join(output, "previous.txt"), "the last good artifact\n")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(t.TempDir(), "govard-home"))
	return root, output
}

// buildDockerStub is a fake `docker` for the whole build command. `inspect`
// answers from GOVARD_TEST_CONTAINER_STATE: "true" and "false" are what the
// daemon prints for a running and a stopped container, and "missing" is the
// daemon's refusal for a container that does not exist. An exec that runs the
// Node preflight prints the tools named in GOVARD_TEST_CONTAINER_MISSING; every
// other exec succeeds without doing anything. Each call is one line in the log.
func buildDockerStub(t *testing.T, state, missingTools string) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "docker.log")
	writeFile(t, filepath.Join(dir, "docker"), `#!/bin/sh
printf '%s\n' "$*" | tr '\n' ' ' >> "$GOVARD_TEST_DOCKER_LOG"
printf '\n' >> "$GOVARD_TEST_DOCKER_LOG"
for last do :; done
case "$1" in
  inspect)
    if [ "$GOVARD_TEST_CONTAINER_STATE" = missing ]; then
      echo "Error: No such object: sample-php-1" >&2
      exit 1
    fi
    echo "$GOVARD_TEST_CONTAINER_STATE"
    ;;
  exec)
    case "$last" in
      *"command -v"*) for tool in $GOVARD_TEST_CONTAINER_MISSING; do echo "$tool"; done ;;
    esac
    ;;
esac
exit 0
`)
	if err := os.Chmod(filepath.Join(dir, "docker"), 0o755); err != nil {
		t.Fatalf("make the docker stub executable: %v", err)
	}
	t.Setenv("GOVARD_TEST_DOCKER_LOG", logFile)
	t.Setenv("GOVARD_TEST_CONTAINER_STATE", state)
	t.Setenv("GOVARD_TEST_CONTAINER_MISSING", missingTools)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func buildDockerCalls(t *testing.T, logFile string) []string {
	t.Helper()
	raw, err := os.ReadFile(logFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the docker log: %v", err)
	}
	var calls []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

// runContainerBuild runs `govard deploy build local` through the root command,
// the entry point an operator uses, and resets the package-level flags after.
func runContainerBuild(t *testing.T, args ...string) error {
	t.Helper()
	t.Cleanup(func() {
		cmd.DeployBuildCommand().Flags().VisitAll(func(flag *pflag.Flag) {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
	})
	command := cmd.RootCommandForTest()
	command.SetArgs(append([]string{"deploy", "build", "local"}, args...))
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	return command.Execute()
}

// A stopped or missing container used to surface only at the first `docker
// exec`, after prepareOutputDir had already deleted the previous artifact, and
// as exit 1 with a hint blaming the revision. The probe comes first now: exit 3,
// the `govard env up` hint, and the last good artifact untouched.
func TestContainerBuildStoppedContainerExits3AndKeepsPreviousArtifact(t *testing.T) {
	for _, state := range []string{"false", "missing"} {
		t.Run("container "+state, func(t *testing.T) {
			stubPresentContainerRuntime(t)
			_, output := containerBuildProject(t, "")
			logFile := buildDockerStub(t, state, "")

			err := runContainerBuild(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef")
			if err == nil {
				t.Fatal("want a refusal when the project container is not running")
			}
			if code := cli.Code(err); code != runtime.CodeCapabilityMissing {
				t.Fatalf("exit code = %d, want %d: %v", code, runtime.CodeCapabilityMissing, err)
			}
			var missing *runtime.MissingError
			if !errors.As(err, &missing) {
				t.Fatalf("error = %v, want *runtime.MissingError", err)
			}
			for _, want := range []string{"govard env up", "--runner host"} {
				if !strings.Contains(missing.Hint, want) {
					t.Errorf("hint %q must name %q", missing.Hint, want)
				}
			}
			if !strings.Contains(missing.Detail, "sample-php-1") {
				t.Errorf("detail %q must name the container", missing.Detail)
			}
			if _, statErr := os.Stat(filepath.Join(output, "previous.txt")); statErr != nil {
				t.Fatalf("the previous artifact was wiped before the container was known to be down: %v", statErr)
			}
			calls := buildDockerCalls(t, logFile)
			if len(calls) == 0 || !strings.HasPrefix(calls[0], "inspect ") {
				t.Fatalf("the container was never probed, docker calls: %q", calls)
			}
			for _, call := range calls {
				if strings.HasPrefix(call, "exec ") {
					t.Fatalf("a build step reached a container that is not running: %q", call)
				}
			}
		})
	}
}

// An --output the container cannot reach is a configuration mistake, and the
// help promises exit 4 for those. Driven through the command, so a wrap that
// only the runner helper applied would not pass.
func TestUnmappableOutputExits4(t *testing.T) {
	stubPresentContainerRuntime(t)
	containerBuildProject(t, "")
	logFile := buildDockerStub(t, "true", "")
	elsewhere := filepath.Join(t.TempDir(), "artifacts")

	err := runContainerBuild(t, "--runner", "container", "--output", elsewhere, "--revision", "deadbeef")
	if err == nil {
		t.Fatal("want a refusal when --output is outside the project root")
	}
	if code := cli.Code(err); code != cli.CodeConfig {
		t.Fatalf("exit code = %d, want %d (configuration): %v", code, cli.CodeConfig, err)
	}
	if !errors.Is(err, deploy.ErrContainerPathUnmappable) {
		t.Fatalf("error = %v, want deploy.ErrContainerPathUnmappable", err)
	}
	if calls := buildDockerCalls(t, logFile); len(calls) > 0 {
		t.Fatalf("a configuration refusal must not reach docker, got %q", calls)
	}
}

// isolateHostPath narrows PATH to the docker stub's directory plus the two tools
// the stub and the shell need, so what the host "has" is exactly what the test
// installs. Without it a developer machine that happens to carry Node would make
// the neither-has-Node case pass for the wrong reason. withNode installs fake
// node and npm that log `npm <cwd> <args>` to the returned file.
func isolateHostPath(t *testing.T, withNode bool) string {
	t.Helper()
	dir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	for _, tool := range []string{"sh", "tr"} {
		found, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("find %s: %v", tool, err)
		}
		if err := os.Symlink(found, filepath.Join(dir, tool)); err != nil {
			t.Fatalf("link %s: %v", tool, err)
		}
	}
	hostLog := filepath.Join(dir, "host.log")
	if withNode {
		writeFile(t, filepath.Join(dir, "node"), "#!/bin/sh\nexit 0\n")
		writeFile(t, filepath.Join(dir, "npm"), "#!/bin/sh\nprintf 'npm %s %s\\n' \"$PWD\" \"$*\" >> '"+hostLog+"'\n")
		for _, name := range []string{"node", "npm"} {
			if err := os.Chmod(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatalf("chmod %s: %v", name, err)
			}
		}
	}
	t.Setenv("PATH", dir)
	return hostLog
}

// runContainerBuildOutput is runContainerBuild with the timeline captured.
func runContainerBuildOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Cleanup(func() {
		cmd.DeployBuildCommand().Flags().VisitAll(func(flag *pflag.Flag) {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
	})
	var out strings.Builder
	command := cmd.RootCommandForTest()
	command.SetArgs(append([]string{"deploy", "build", "local"}, args...))
	command.SetOut(&out)
	command.SetErr(io.Discard)
	err := command.Execute()
	return out.String(), err
}

// Neither the container nor the host has Node: the only case that is still
// refused. The refusal comes before the first task and before the previous
// artifact is cleared, and names both sides and the ways out.
func TestContainerBuildRefusesNodeStepWhenNeitherSideHasNode(t *testing.T) {
	stubPresentContainerRuntime(t)
	_, output := containerBuildProject(t, "resources")
	logFile := buildDockerStub(t, "true", "npm")
	isolateHostPath(t, false)

	err := runContainerBuild(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef")
	if err == nil {
		t.Fatal("want a refusal when neither the container nor the host has Node")
	}
	if code := cli.Code(err); code != runtime.CodeCapabilityMissing {
		t.Fatalf("exit code = %d, want %d: %v", code, runtime.CodeCapabilityMissing, err)
	}
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want *runtime.MissingError", err)
	}
	for _, want := range []string{"npm", "build:frontend", "sample-php-1", "host"} {
		if !strings.Contains(missing.Detail, want) {
			t.Errorf("detail %q must name %q", missing.Detail, want)
		}
	}
	// Execute prints err.Error() and nothing else outside --error-json, so the
	// way out has to be in the text itself, not only in the envelope's hint.
	for _, want := range []string{"frontend_dir", "container", "host"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the printed error %q must name %q", err.Error(), want)
		}
	}
	if missing.Hint == "" {
		t.Error("the refusal carries no hint")
	}
	if _, statErr := os.Stat(filepath.Join(output, "previous.txt")); statErr != nil {
		t.Fatalf("the previous artifact was wiped before the refusal: %v", statErr)
	}
	var execs []string
	for _, call := range buildDockerCalls(t, logFile) {
		if strings.HasPrefix(call, "exec ") {
			execs = append(execs, call)
		}
	}
	if len(execs) != 1 || !strings.Contains(execs[0], "command -v") {
		t.Fatalf("want the preflight as the only exec, before any build task, got %q", execs)
	}
}

// The mixed runner (#531): the container has no Node but the host does, so the
// PHP steps stay in the container, the frontend step runs on the host, the
// timeline says so per step, and the manifest records which runner built it.
func TestContainerBuildRunsTheFrontendStepOnTheHostWhenTheContainerHasNoNode(t *testing.T) {
	stubPresentContainerRuntime(t)
	_, output := containerBuildProject(t, ".")
	logFile := buildDockerStub(t, "true", "node npm")
	hostLog := isolateHostPath(t, true)

	timeline, err := runContainerBuildOutput(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	frontend := timelineBlock(timeline, "build:frontend")
	if !strings.Contains(frontend, "runner: host (node not in container)") {
		t.Errorf("the build:frontend entry must name the host runner, got %q in:\n%s", frontend, timeline)
	}
	vendors := timelineBlock(timeline, "build:vendors")
	if !strings.Contains(vendors, "runner: container sample-php-1") {
		t.Errorf("the build:vendors entry must name the container runner, got %q in:\n%s", vendors, timeline)
	}

	for _, call := range buildDockerCalls(t, logFile) {
		if strings.Contains(call, "npm") && !strings.Contains(call, "command -v") {
			t.Errorf("a Node command reached the container that has no Node: %q", call)
		}
	}
	raw, readErr := os.ReadFile(hostLog)
	if readErr != nil {
		t.Fatalf("the host never ran npm: %v", readErr)
	}
	if !strings.Contains(string(raw), " ci\n") || !strings.Contains(string(raw), " run build\n") {
		t.Errorf("host npm log = %q, want ci and run build", raw)
	}
	// The docker stub materialises nothing, so the frontend directory is the
	// artifact root itself: the host step must run inside the artifact tree.
	if !strings.Contains(string(raw), output) {
		t.Errorf("host npm ran in %q, want it inside the artifact %s", raw, output)
	}

	manifest, err := deploy.ReadManifest(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if manifest.FrontendRunner != "host" {
		t.Errorf("manifest frontend_runner = %q, want host", manifest.FrontendRunner)
	}
}

// A container that has Node keeps running the frontend in the container, and the
// host is not consulted at all: its Node, or the lack of it, must not matter.
func TestContainerBuildKeepsTheFrontendStepInTheContainerWhenItHasNode(t *testing.T) {
	stubPresentContainerRuntime(t)
	_, output := containerBuildProject(t, "resources")
	logFile := buildDockerStub(t, "true", "")
	hostLog := isolateHostPath(t, true)

	timeline, err := runContainerBuildOutput(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	frontend := timelineBlock(timeline, "build:frontend")
	if !strings.Contains(frontend, "runner: container sample-php-1") {
		t.Errorf("the build:frontend entry must name the container runner, got %q", frontend)
	}
	if strings.Contains(timeline, "runner: host") {
		t.Errorf("no step may run on the host, got:\n%s", timeline)
	}
	if _, statErr := os.Stat(hostLog); statErr == nil {
		t.Error("the host ran npm although the container has Node")
	}
	ran := false
	for _, call := range buildDockerCalls(t, logFile) {
		if strings.Contains(call, "npm ci") {
			ran = true
		}
	}
	if !ran {
		t.Error("the frontend step never ran in the container")
	}
	manifest, err := deploy.ReadManifest(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if manifest.FrontendRunner != "container" {
		t.Errorf("manifest frontend_runner = %q, want container", manifest.FrontendRunner)
	}
}

// With no frontend directory the step never runs, so there is nothing to record
// and no runner line for it.
func TestContainerBuildRecordsNoFrontendRunnerWithoutAFrontendStep(t *testing.T) {
	stubPresentContainerRuntime(t)
	_, output := containerBuildProject(t, "")
	buildDockerStub(t, "true", "node npm")
	isolateHostPath(t, false)

	if _, err := runContainerBuildOutput(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef"); err != nil {
		t.Fatalf("build: %v", err)
	}
	manifest, err := deploy.ReadManifest(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if manifest.FrontendRunner != "" {
		t.Errorf("manifest frontend_runner = %q, want empty", manifest.FrontendRunner)
	}
}

// timelineBlock returns a step's arrow line plus the indented lines under it.
func timelineBlock(timeline, stepID string) string {
	lines := strings.Split(timeline, "\n")
	for index, line := range lines {
		if !strings.Contains(line, "→ "+stepID) {
			continue
		}
		block := []string{line}
		for _, next := range lines[index+1:] {
			if strings.HasPrefix(next, "    ") {
				block = append(block, next)
				continue
			}
			break
		}
		return strings.Join(block, "\n")
	}
	return ""
}

// The preflight is gated on the frontend step actually running: a project with
// no frontend directory never runs npm, so a PHP-only container must build it,
// and a container that has Node passes the preflight and goes on to the tasks.
func TestContainerBuildNodePreflightOnlyWhenTheFrontendStepRuns(t *testing.T) {
	cases := []struct {
		name          string
		frontendDirs  string
		missing       string
		wantPreflight bool
	}{
		{name: "no frontend directory skips the probe", frontendDirs: "", missing: "node npm", wantPreflight: false},
		{name: "a container with Node passes it", frontendDirs: "resources", missing: "", wantPreflight: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubPresentContainerRuntime(t)
			_, output := containerBuildProject(t, testCase.frontendDirs)
			logFile := buildDockerStub(t, "true", testCase.missing)

			if err := runContainerBuild(t, "--runner", "container", "--output", output, "--force", "--revision", "deadbeef"); err != nil {
				t.Fatalf("build: %v", err)
			}
			preflight, tasks := false, 0
			for _, call := range buildDockerCalls(t, logFile) {
				switch {
				case strings.Contains(call, "command -v"):
					preflight = true
				case strings.HasPrefix(call, "exec "):
					tasks++
				}
			}
			if preflight != testCase.wantPreflight {
				t.Fatalf("preflight ran = %v, want %v", preflight, testCase.wantPreflight)
			}
			if tasks == 0 {
				t.Fatal("the build never reached its tasks")
			}
		})
	}
}
