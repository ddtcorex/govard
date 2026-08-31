package engine

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ComposeOptions defines the parameters for running a Docker Compose command.
type ComposeOptions struct {
	ProjectDir  string
	ProjectName string
	ComposeFile string
	Args        []string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
}

// RunCompose executes a Docker Compose command with the given options.
// It automatically handles project-level flags like --project-directory, -p, and -f.
func RunCompose(ctx context.Context, opts ComposeOptions) error {
	dockerArgs := buildComposeArgs(opts.ProjectDir, opts.ProjectName, opts.ComposeFile, opts.Args)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Dir = opts.ProjectDir

	// Default to standard streams if not provided
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	return cmd.Run()
}

// buildComposeArgs constructs the full argument list for a docker compose command.
func buildComposeArgs(projectDir, projectName, composeFile string, args []string) []string {
	dockerArgs := []string{
		"compose",
		"--project-directory", filepath.Clean(projectDir),
	}

	if projectName != "" {
		dockerArgs = append(dockerArgs, "-p", projectName)
	}

	if composeFile != "" {
		dockerArgs = append(dockerArgs, "-f", composeFile)
	}

	dockerArgs = append(dockerArgs, args...)
	return dockerArgs
}
