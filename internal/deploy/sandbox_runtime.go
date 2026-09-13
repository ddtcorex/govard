package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// SandboxCommand is one container-runtime invocation.
type SandboxCommand struct {
	// Args are the arguments after the runtime binary. No shell is involved.
	Args []string
	// Stdin is fed to the process, for `exec -i` writes.
	Stdin []byte
	// Out receives the process's combined output when the caller wants to see
	// progress (an image build) rather than only the result.
	Out io.Writer
}

// SandboxCommandRunner executes one SandboxCommand and returns its stdout.
//
// It is the seam that makes the sandbox testable: production runs the Docker
// CLI, tests record the arguments and answer from a script. Everything above it
// is ordinary logic about what a deployment target looks like.
type SandboxCommandRunner func(ctx context.Context, command SandboxCommand) (string, error)

// SandboxRuntime is the container-runtime boundary the sandbox command uses.
//
// It is deliberately verbs-shaped rather than a general Docker API: each method
// is one thing the sandbox needs to know about the container it owns, so a test
// doubles the whole boundary in a page and the production implementation has no
// decisions in it.
type SandboxRuntime interface {
	Available(ctx context.Context) error
	ImageExists(ctx context.Context, image string) (bool, error)
	BuildImage(ctx context.Context, request SandboxBuildRequest) error
	RemoveImage(ctx context.Context, image string) error
	ImageFile(ctx context.Context, image, path string) (string, error)

	RunContainer(ctx context.Context, request SandboxRunRequest) error
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string) error
	RemoveContainer(ctx context.Context, name string) error
	ContainerRunning(ctx context.Context, name string) (bool, error)
	ContainerExists(ctx context.Context, name string) (bool, error)
	ContainerImage(ctx context.Context, name string) (string, error)
	ContainerLabel(ctx context.Context, name, label string) (string, error)
	PublishedPort(ctx context.Context, name string, containerPort int) (int, error)
	Exec(ctx context.Context, name string, stdin []byte, args ...string) (string, error)
}

// SandboxBuildRequest is one image build.
type SandboxBuildRequest struct {
	Image      string
	Dockerfile string
	Context    string
	Out        io.Writer
}

// SandboxRunRequest is one container creation. The host port is not here on
// purpose: Docker picks it (see SandboxPortBinding).
type SandboxRunRequest struct {
	Name        string
	Image       string
	MirrorPath  string
	ProjectName string
	Profile     string
	// Web publishes the HTTP port as well. A profile with no web tier has
	// nothing listening there, and Docker would publish a port that never
	// answers — which `deploy:verify` would then report as a failed deploy.
	Web bool
}

// SandboxWebPort is the container port the web tier listens on, readable so a
// caller does not have to know it is 80.
func SandboxWebPort() int { return sandboxWebPort }

// SandboxPortBinding is the loopback binding the sandbox sshd is published on.
// The empty host port means "Docker, choose one" — the port is then read back
// with PublishedPort. A guessed port collides with whatever else the developer
// is running, which turns a regression suite into a flaky one.
const SandboxPortBinding = "127.0.0.1::22"

// DockerCLI is the production SandboxRuntime.
type DockerCLI struct {
	run SandboxCommandRunner
}

// NewDockerCLI returns a runtime that drives the Docker CLI.
func NewDockerCLI() *DockerCLI {
	return &DockerCLI{run: execSandboxCommand("docker")}
}

// NewDockerCLIForTest builds a runtime around an injectable runner.
func NewDockerCLIForTest(run SandboxCommandRunner) *DockerCLI {
	return &DockerCLI{run: run}
}

// execSandboxCommand runs a binary with an argument array. There is deliberately
// no shell: a project name or a path never becomes shell syntax.
func execSandboxCommand(binary string) SandboxCommandRunner {
	return func(ctx context.Context, command SandboxCommand) (string, error) {
		cmd := exec.CommandContext(ctx, binary, command.Args...)
		cmd.WaitDelay = waitDelayAfterKill

		var stdout, stderr bytes.Buffer
		if command.Out != nil {
			cmd.Stdout = io.MultiWriter(&stdout, command.Out)
			cmd.Stderr = io.MultiWriter(&stderr, command.Out)
		} else {
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
		}
		if len(command.Stdin) > 0 {
			cmd.Stdin = bytes.NewReader(command.Stdin)
		}

		err := cmd.Run()
		if err == nil {
			return stdout.String(), nil
		}
		full := strings.TrimSpace(binary + " " + strings.Join(command.Args, " "))
		return stdout.String(), commandError(ctx, full, stderr.String(), err)
	}
}

// Available reports whether a container runtime is reachable at all. It is the
// difference between "start Docker" and "the sandbox is broken".
func (d *DockerCLI) Available(ctx context.Context) error {
	if _, err := d.run(ctx, SandboxCommand{Args: []string{"version", "--format", "{{.Server.Version}}"}}); err != nil {
		return fmt.Errorf("no reachable container runtime: %w", err)
	}
	return nil
}

// ImageExists reports whether an image is already local. A missing image is not
// an error: it is the normal state before the first build.
func (d *DockerCLI) ImageExists(ctx context.Context, image string) (bool, error) {
	_, err := d.run(ctx, SandboxCommand{Args: []string{"image", "inspect", "--format", "{{.Id}}", image}})
	if err == nil {
		return true, nil
	}
	var commandErr *CommandError
	if errors.As(err, &commandErr) {
		// `docker image inspect` exits non-zero for an absent image and for a
		// stopped daemon alike; only the daemon case is worth failing on, and
		// Available already asked that question.
		return false, nil
	}
	return false, err
}

// BuildImage builds one image from the rendered Dockerfile.
func (d *DockerCLI) BuildImage(ctx context.Context, request SandboxBuildRequest) error {
	_, err := d.run(ctx, SandboxCommand{
		Args: []string{"build", "--file", request.Dockerfile, "--tag", request.Image, request.Context},
		Out:  request.Out,
	})
	if err != nil {
		return fmt.Errorf("build the sandbox image %s: %w", request.Image, err)
	}
	return nil
}

// RemoveImage removes a local image, for `down --purge`.
func (d *DockerCLI) RemoveImage(ctx context.Context, image string) error {
	_, err := d.run(ctx, SandboxCommand{Args: []string{"image", "rm", "--force", image}})
	if err != nil {
		return fmt.Errorf("remove the sandbox image %s: %w", image, err)
	}
	return nil
}

// ImageFile reads one file out of an image without starting the sandbox, which
// is how `status` reports the package versions the image actually recorded even
// while the container is stopped.
func (d *DockerCLI) ImageFile(ctx context.Context, image, path string) (string, error) {
	output, err := d.run(ctx, SandboxCommand{Args: []string{"run", "--rm", "--entrypoint", "cat", image, path}})
	if err != nil {
		return "", fmt.Errorf("read %s from %s: %w", path, image, err)
	}
	return output, nil
}

// RunContainer creates and starts the sandbox container.
func (d *DockerCLI) RunContainer(ctx context.Context, request SandboxRunRequest) error {
	args := []string{
		"run", "--detach",
		"--name", request.Name,
		"--label", "govard.sandbox=1",
		"--label", "govard.sandbox.project=" + request.ProjectName,
		"--label", "govard.sandbox.profile=" + request.Profile,
		"--publish", SandboxPortBinding,
	}
	if request.Web {
		args = append(args, "--publish", SandboxWebBinding)
	}
	args = append(args,
		"--mount", "type=bind,source="+request.MirrorPath+",target="+SandboxRepoPath+",readonly",
		request.Image,
	)
	if _, err := d.run(ctx, SandboxCommand{Args: args}); err != nil {
		return fmt.Errorf("start the sandbox container %s: %w", request.Name, err)
	}
	return nil
}

// StartContainer restarts a container that already exists.
func (d *DockerCLI) StartContainer(ctx context.Context, name string) error {
	if _, err := d.run(ctx, SandboxCommand{Args: []string{"start", name}}); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	return nil
}

// StopContainer stops a running container. Stopping an already stopped one is
// not an error, so the caller does not have to ask first.
func (d *DockerCLI) StopContainer(ctx context.Context, name string) error {
	if _, err := d.run(ctx, SandboxCommand{Args: []string{"stop", name}}); err != nil {
		return fmt.Errorf("stop %s: %w", name, err)
	}
	return nil
}

// RemoveContainer removes a container, running or not.
func (d *DockerCLI) RemoveContainer(ctx context.Context, name string) error {
	if _, err := d.run(ctx, SandboxCommand{Args: []string{"rm", "--force", "--volumes", name}}); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}

// ContainerRunning reports whether the container exists and is running.
func (d *DockerCLI) ContainerRunning(ctx context.Context, name string) (bool, error) {
	output, err := d.run(ctx, SandboxCommand{Args: []string{"inspect", "--format", "{{.State.Running}}", name}})
	if err != nil {
		var commandErr *CommandError
		if errors.As(err, &commandErr) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(output) == "true", nil
}

// ContainerExists reports whether a container with this name exists at all,
// running or stopped. `up` must reuse a stopped sandbox, not fight it for the
// name.
func (d *DockerCLI) ContainerExists(ctx context.Context, name string) (bool, error) {
	_, err := d.run(ctx, SandboxCommand{Args: []string{"inspect", "--format", "{{.Id}}", name}})
	if err == nil {
		return true, nil
	}
	var commandErr *CommandError
	if errors.As(err, &commandErr) {
		return false, nil
	}
	return false, err
}

// ContainerImage reports the image a container was created from.
func (d *DockerCLI) ContainerImage(ctx context.Context, name string) (string, error) {
	output, err := d.run(ctx, SandboxCommand{Args: []string{"inspect", "--format", "{{.Config.Image}}", name}})
	if err != nil {
		return "", fmt.Errorf("read the image of %s: %w", name, err)
	}
	return strings.TrimSpace(output), nil
}

// ContainerLabel reads one label off a container.
func (d *DockerCLI) ContainerLabel(ctx context.Context, name, label string) (string, error) {
	format := fmt.Sprintf("{{index .Config.Labels %q}}", label)
	output, err := d.run(ctx, SandboxCommand{Args: []string{"inspect", "--format", format, name}})
	if err != nil {
		return "", fmt.Errorf("read label %s of %s: %w", label, name, err)
	}
	return strings.TrimSpace(output), nil
}

// PublishedPort reads back the host port Docker chose for a container port.
func (d *DockerCLI) PublishedPort(ctx context.Context, name string, containerPort int) (int, error) {
	spec := strconv.Itoa(containerPort) + "/tcp"
	output, err := d.run(ctx, SandboxCommand{Args: []string{"port", name, spec}})
	if err != nil {
		return 0, fmt.Errorf("read the published port of %s: %w", name, err)
	}
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		_, portText, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(portText))
		if err != nil || port <= 0 {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("docker published no host port for %s in %q", spec, strings.TrimSpace(output))
}

// Exec runs one command inside the container, optionally feeding it stdin
// (`docker exec --interactive <name> <args...>`).
func (d *DockerCLI) Exec(ctx context.Context, name string, stdin []byte, args ...string) (string, error) {
	execArgs := make([]string, 0, len(args)+3)
	execArgs = append(execArgs, "exec")
	if len(stdin) > 0 {
		execArgs = append(execArgs, "--interactive")
	}
	execArgs = append(execArgs, name)
	execArgs = append(execArgs, args...)

	output, err := d.run(ctx, SandboxCommand{Args: execArgs, Stdin: stdin})
	if err != nil {
		return "", fmt.Errorf("exec %s in %s: %w", strings.Join(args, " "), name, err)
	}
	return output, nil
}
