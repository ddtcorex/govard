package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DockerClient is the provider-neutral Docker boundary used by audit
// toolchains and explicitly configured external lint providers.
type DockerClient interface {
	Pull(context.Context, string) error
	Inspect(context.Context, string) (ImageInspection, error)
	Build(context.Context, string, string, map[string]string) error
	Run(context.Context, ContainerRunRequest, io.Writer) error
	Stop(context.Context, string, time.Duration) error
	Remove(context.Context, string) error
}

type ImageInspection struct {
	ID          string
	RepoDigests []string
	Labels      map[string]string
}

type ContainerMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ContainerRunRequest struct {
	Name  string
	Image string
	// User is the "<uid>[:<gid>]" identity the container process runs as. It
	// stays empty when the caller has no host identity to impose, so Docker
	// keeps the image's own user.
	User        string
	Args        []string
	Mounts      []ContainerMount
	Environment map[string]string
	Labels      map[string]string
	AutoRemove  bool
}

type DockerRunCommand func(context.Context, string, []string, io.Writer, io.Writer) error

// ExecDockerClient runs the Docker CLI through an injectable argument-array
// executor. It deliberately has no shell execution path.
type ExecDockerClient struct {
	binary string
	run    DockerRunCommand
}

func NewExecDockerClient(run DockerRunCommand) *ExecDockerClient {
	client := &ExecDockerClient{binary: "docker", run: run}
	if client.run == nil {
		client.run = func(ctx context.Context, binary string, args []string, stdout, stderr io.Writer) error {
			command := exec.CommandContext(ctx, binary, args...)
			command.Stdout = stdout
			command.Stderr = stderr
			return command.Run()
		}
	}
	return client
}

func (client *ExecDockerClient) Pull(ctx context.Context, image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("docker pull image is empty")
	}
	return client.run(ctx, client.binary, []string{"pull", image}, io.Discard, io.Discard)
}

func (client *ExecDockerClient) Inspect(ctx context.Context, image string) (ImageInspection, error) {
	if strings.TrimSpace(image) == "" {
		return ImageInspection{}, fmt.Errorf("docker inspect image is empty")
	}
	var output bytes.Buffer
	if err := client.run(ctx, client.binary, []string{"image", "inspect", "--format={{json .}}", image}, &output, &output); err != nil {
		return ImageInspection{}, err
	}
	var raw struct {
		ID          string   `json:"Id"`
		RepoDigests []string `json:"RepoDigests"`
		Config      struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output.Bytes(), &raw); err != nil {
		return ImageInspection{}, fmt.Errorf("decode Docker image inspection: %w", err)
	}
	return ImageInspection{ID: raw.ID, RepoDigests: raw.RepoDigests, Labels: raw.Config.Labels}, nil
}

func (client *ExecDockerClient) Build(ctx context.Context, contextDir, image string, labels map[string]string) error {
	if strings.TrimSpace(contextDir) == "" || strings.TrimSpace(image) == "" {
		return fmt.Errorf("docker build requires context and image")
	}
	args := []string{"build", "--tag", image}
	for _, key := range sortedMapKeys(labels) {
		args = append(args, "--label", key+"="+labels[key])
	}
	args = append(args, contextDir)
	return client.run(ctx, client.binary, args, io.Discard, io.Discard)
}

func (client *ExecDockerClient) Run(ctx context.Context, request ContainerRunRequest, output io.Writer) error {
	if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Image) == "" || len(request.Args) == 0 {
		return fmt.Errorf("docker run requires name, image, and command")
	}
	args := []string{"run", "--name", request.Name}
	if user := strings.TrimSpace(request.User); user != "" {
		args = append(args, "--user", user)
	}
	if request.AutoRemove {
		args = append(args, "--rm")
	}
	for _, mount := range request.Mounts {
		if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Target) == "" {
			return fmt.Errorf("docker run contains an incomplete mount")
		}
		value := mount.Source + ":" + mount.Target
		if mount.ReadOnly {
			value += ":ro"
		}
		args = append(args, "-v", value)
	}
	for _, key := range sortedMapKeys(request.Environment) {
		args = append(args, "-e", key+"="+request.Environment[key])
	}
	for _, key := range sortedMapKeys(request.Labels) {
		args = append(args, "--label", key+"="+request.Labels[key])
	}
	args = append(args, request.Image)
	args = append(args, request.Args...)
	return client.run(ctx, client.binary, args, output, output)
}

func (client *ExecDockerClient) Stop(ctx context.Context, name string, timeout time.Duration) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("docker stop name is empty")
	}
	seconds := int(timeout.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return client.run(ctx, client.binary, []string{"stop", "--time", strconv.Itoa(seconds), name}, io.Discard, io.Discard)
}

func (client *ExecDockerClient) Remove(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("docker remove name is empty")
	}
	return client.run(ctx, client.binary, []string{"rm", "--force", name}, io.Discard, io.Discard)
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
