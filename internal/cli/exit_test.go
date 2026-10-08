package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

type coded struct{ code int }

func (c coded) Error() string { return "coded" }
func (c coded) ExitCode() int { return c.code }

func TestCodeDefaultsToExecutionError(t *testing.T) {
	if got := Code(errors.New("boom")); got != CodeError {
		t.Fatalf("Code = %d, want %d", got, CodeError)
	}
}

func TestCodeIsZeroForNil(t *testing.T) {
	if got := Code(nil); got != CodeOK {
		t.Fatalf("Code(nil) = %d, want %d", got, CodeOK)
	}
}

func TestCodeReadsWrappedExitCode(t *testing.T) {
	err := fmt.Errorf("cannot run %q: %w", "env up", coded{code: 3})
	if got := Code(err); got != 3 {
		t.Fatalf("Code = %d, want 3", got)
	}
}

func TestUsageErrorReportsTwo(t *testing.T) {
	if got := Code(&UsageError{Err: errors.New("unknown flag")}); got != CodeUsage {
		t.Fatalf("Code = %d, want %d", got, CodeUsage)
	}
}

// Exit code 4 was reserved in the capability contract and had no producer: every
// configuration failure was reported as a usage error, which sent the operator
// to the command line instead of the file.
func TestConfigErrorReportsFour(t *testing.T) {
	if got := Code(&ConfigError{Err: errors.New("deploy.verify.timeout: bad")}); got != CodeConfig {
		t.Fatalf("Code = %d, want %d", got, CodeConfig)
	}
	wrapped := fmt.Errorf("deploy local: %w", &ConfigError{Err: errors.New("no branch configured")})
	if got := Code(wrapped); got != CodeConfig {
		t.Fatalf("Code(wrapped) = %d, want %d", got, CodeConfig)
	}
}

func TestConfigErrorCarriesTheConfigEnvelopeCode(t *testing.T) {
	envelope := NewErrorEnvelope("govard deploy", &ConfigError{Err: errors.New("bad value")})
	if envelope.Error.Code != CodeConfigName {
		t.Fatalf("envelope code = %q, want %q", envelope.Error.Code, CodeConfigName)
	}
}

// A failed child process is not a govard result: its status (255 from ssh, 22
// from curl, 127 for a missing binary) is whatever the remote command chose, and
// only 0 to 4 are documented. The child's code stays in the message.
func TestCodeIgnoresAChildProcessExitStatus(t *testing.T) {
	for _, status := range []int{255, 22, 127, 4, 3} {
		child := exec.Command("sh", "-c", fmt.Sprintf("exit %d", status)).Run()
		var exitErr *exec.ExitError
		if !errors.As(child, &exitErr) {
			t.Fatalf("status %d: want an ExitError, got %v", status, child)
		}
		wrapped := fmt.Errorf("step db:migrate failed: %w", fmt.Errorf("command failed (exit %d): x: %w", status, child))
		if got := Code(wrapped); got != CodeError {
			t.Fatalf("exit %d: Code = %d, want %d", status, got, CodeError)
		}
		if got := NewErrorEnvelope("govard deploy", wrapped).Error.Code; got != CodeErrorName {
			t.Fatalf("exit %d: envelope code = %q, want %q", status, got, CodeErrorName)
		}
	}
}

func TestCodeKeepsAGovardCodeBeneathAChildFailure(t *testing.T) {
	child := exec.Command("sh", "-c", "exit 255").Run()
	err := fmt.Errorf("outer: %w", errors.Join(child, &ConfigError{Err: errors.New("bad")}))
	if got := Code(err); got != CodeConfig {
		t.Fatalf("Code = %d, want %d", got, CodeConfig)
	}
}
