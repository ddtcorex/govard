package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"govard/internal/deploy"
)

// cleanupProjectSandbox removes what a project's sandbox left behind: the
// container, the database volume, the image tags built for exactly this project
// and the state directory (key, mirror, Dockerfile). Every step is best effort:
// a failure is a warning, because the project delete must still finish.
func cleanupProjectSandbox(ctx context.Context, name, root string, stdout, stderr io.Writer) {
	root = strings.TrimSpace(root)
	if root == "" || strings.TrimSpace(name) == "" {
		return
	}
	runtime := deploy.NewDockerCLI()
	if err := runtime.Available(ctx); err != nil {
		return
	}

	request := deploy.SandboxRequest{ProjectRoot: root, ProjectName: name, Purge: true, Volumes: true, Out: stdout}
	if _, err := deploy.SandboxDown(ctx, runtime, request); err != nil {
		fmt.Fprintf(stderr, "Warning: sandbox cleanup: %v\n", err)
	}

	// Residue SandboxDown cannot reach once the container is gone: the database
	// volume is found by its exact container-name label, the images by the exact
	// tag shape built for this project. No --force: an image another checkout
	// still runs is left alone.
	container := deploy.SandboxContainerName(name, root)
	if err := runtime.RemoveVolumesByLabel(ctx, deploy.SandboxDBVolumeLabel, container); err != nil {
		fmt.Fprintf(stderr, "Warning: sandbox database volume: %v\n", err)
	}
	listed, err := exec.CommandContext(ctx, "docker", "image", "ls", "--format", "{{.Repository}}:{{.Tag}}", "govard-sandbox").Output()
	if err != nil {
		return
	}
	for _, tag := range deploy.SandboxProjectImageTags(name, strings.Fields(string(listed))) {
		rm := exec.CommandContext(ctx, "docker", "image", "rm", tag)
		rm.Stdout = stdout
		rm.Stderr = stderr
		if err := rm.Run(); err != nil {
			fmt.Fprintf(stderr, "Warning: could not remove sandbox image %s: %v\n", tag, err)
		}
	}
	// SandboxDown removes the state directory only when it ran far enough.
	if err := os.RemoveAll(deploy.SandboxStateDir(root)); err != nil {
		fmt.Fprintf(stderr, "Warning: sandbox state directory: %v\n", err)
	}
}
