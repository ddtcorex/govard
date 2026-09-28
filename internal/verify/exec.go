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

// EnvBinaryOverride pins the binary the checklist runs. Set it when validating
// a source build: without it the invoked executable wins, which is the point —
// resolving through PATH made `./bin/govard verify` silently test whatever
// `govard` the shell found first (issue #464).
const EnvBinaryOverride = "GOVARD_VERIFY_BIN"

// execGovardFake is a test hook: when set, execGovard returns this value without spawning a process.
var execGovardFake func(ctx context.Context, cfg engine.Config, opts VerifyOpts, args ...string) (Evidence, bool)

// SetExecGovardFakeForTest installs a fake for hermetic tests.
func SetExecGovardFakeForTest(fn func(ctx context.Context, cfg engine.Config, opts VerifyOpts, args ...string) (Evidence, bool)) {
	execGovardFake = fn
}

// ProjectRoot is set by cmd/verify.go via VerifyOpts.ProjectRoot.
func execGovard(ctx context.Context, cfg engine.Config, opts VerifyOpts, args ...string) Evidence {
	if execGovardFake != nil {
		if ev, ok := execGovardFake(ctx, cfg, opts, args...); ok {
			return ev
		}
	}
	if os.Getenv("GOVARD_VERIFY_FAKE") == "1" {
		return Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " "), JSONValid: false}
	}
	bin := govardBinary()
	if isTestBinary(bin) {
		return Evidence{ExitCode: 0, OutputExcerpt: "fake(test-binary): " + strings.Join(args, " "), JSONValid: false}
	}
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

// govardBinary resolves the binary an item runs: an explicit override first,
// then the executable that is already running, and PATH only as a last resort.
// Preferring PATH here was the bug — a source build validated the installed CLI.
func govardBinary() string {
	if p := strings.TrimSpace(os.Getenv(EnvBinaryOverride)); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	if p, err := exec.LookPath("govard"); err == nil {
		return p
	}
	return "govard"
}

// GovardBinaryForTest exposes govardBinary for tests in /tests.
func GovardBinaryForTest() string { return govardBinary() }

func isTestBinary(bin string) bool {
	if strings.Contains(bin, ".test") {
		return true
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if strings.Contains(exe, ".test") {
			return true
		}
		if bin == exe && strings.HasSuffix(exe, ".test") {
			return true
		}
	}
	// Also treat go test binary invocation as test.
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	return false
}
