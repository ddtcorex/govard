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
