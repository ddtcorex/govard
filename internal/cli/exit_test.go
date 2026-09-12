package cli

import (
	"errors"
	"fmt"
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
