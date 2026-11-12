package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

	// What compose prints on stderr is the diagnosis of a failure, and the caller's
	// stream gets it either way. A bounded tail is kept as well, so the returned
	// error can say why the run failed instead of only "exit status 1".
	tail := &tailBuffer{limit: composeErrorTailBytes}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	cmd.Stderr = io.MultiWriter(stderr, tail)

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	if err := cmd.Run(); err != nil {
		if detail := tail.lastLine(); detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

// composeErrorTailBytes bounds how much of compose's stderr is kept for the error
// message. The last line is what names the cause; the buffer only has to survive
// output that arrives without newlines.
const composeErrorTailBytes = 8 << 10

// tailBuffer keeps the most recent bytes written to it.
type tailBuffer struct {
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

// lastLine is the last non-empty line of what was captured, without ANSI escapes
// and bounded in length: an error message ends up in a terminal and in a JSON
// envelope alike.
func (b *tailBuffer) lastLine() string {
	text := strings.TrimSpace(sanitizeComposeDetail(string(b.data)))
	if text == "" {
		return ""
	}
	if index := strings.LastIndexByte(text, '\n'); index >= 0 {
		text = text[index+1:]
	}
	text = strings.TrimSpace(text)
	if len(text) > composeErrorDetailMax {
		text = text[:composeErrorDetailMax] + "..."
	}
	return text
}

// composeErrorDetailMax keeps an envelope message readable.
const composeErrorDetailMax = 240

// composeEscapePattern matches the escapes docker prints when it decides it is
// talking to a terminal; the detail is copied into logs and JSON too.
var composeEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

func sanitizeComposeDetail(text string) string {
	return composeEscapePattern.ReplaceAllString(text, "")
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
