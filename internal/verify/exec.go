package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	bin := govardBinary()
	// Both fake paths are hermetic-test hooks. They fire only when the binary
	// that would run is a go test binary, so a production run can never be
	// turned green by a stray GOVARD_VERIFY_FAKE=1 (issue #519).
	if isTestBinary(bin) {
		prefix := "fake(test-binary): "
		if os.Getenv("GOVARD_VERIFY_FAKE") == "1" {
			prefix = "fake: "
		}
		return Evidence{ExitCode: 0, OutputExcerpt: prefix + strings.Join(args, " "), JSONValid: false, Fake: true}
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
	excerpt := excerptOf(out, exitCode)
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

// isTestBinary reports whether bin is a go test binary. Only the basename
// counts: a directory such as govard.test/ or a name such as x.testing must not
// match, or a real install path would fake every result.
func isTestBinary(bin string) bool {
	return strings.HasSuffix(filepath.Base(bin), ".test")
}

// IsTestBinaryForTest exposes isTestBinary for tests in /tests.
func IsTestBinaryForTest(bin string) bool { return isTestBinary(bin) }

// ExecGovardForTest exposes execGovard for tests in /tests.
func ExecGovardForTest(ctx context.Context, cfg engine.Config, opts VerifyOpts, args ...string) Evidence {
	return execGovard(ctx, cfg, opts, args...)
}

// excerptLimit bounds the evidence kept per item.
const excerptLimit = 500

// excerptOf bounds a child's output. A successful run keeps its head (the JSON
// identity lines consumers read come first). A failing run keeps head and tail
// with a marker between, because the actual error is the last thing printed.
func excerptOf(out string, exitCode int) string {
	excerpt := strings.TrimSpace(out)
	if len(excerpt) <= excerptLimit {
		return excerpt
	}
	if exitCode == 0 {
		return excerpt[:excerptLimit]
	}
	const marker = "\n[... output truncated ...]\n"
	half := excerptLimit / 2
	return excerpt[:half] + marker + excerpt[len(excerpt)-half:]
}

// ExcerptForTest exposes excerptOf to the tests package.
func ExcerptForTest(out string, exitCode int) string { return excerptOf(out, exitCode) }
