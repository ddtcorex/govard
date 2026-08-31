package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"
)

// ProjectRoot is set by cmd/verify.go via VerifyOpts.ProjectRoot.
func execGovard(ctx context.Context, cfg engine.Config, opts VerifyOpts, args ...string) Evidence {
	bin := govardBinary()
	start := time.Now()
	// Build command — run from ProjectRoot via cmd.Dir, not --project flag (most govard commands don't have it)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	if opts.ProjectRoot != "" {
		cmd.Dir = opts.ProjectRoot
	}
	// Capture combined
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	dur := time.Since(start)
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 1
		}
	}
	out := buf.String()
	// Truncate excerpt to 500 chars
	excerpt := strings.TrimSpace(out)
	if len(excerpt) > 500 {
		excerpt = excerpt[:500]
	}
	// JSON valid if output is JSON (for --json cases)
	jsonValid := json.Valid([]byte(strings.TrimSpace(out)))
	_ = dur
	_ = cfg
	return Evidence{
		ExitCode:      exitCode,
		OutputExcerpt: excerpt,
		JSONValid:     jsonValid,
	}
}

func govardBinary() string {
	if p, err := exec.LookPath("govard"); err == nil {
		return p
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "govard"
}
