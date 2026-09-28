package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/verify"
)

func TestGovardBinaryPrefersRunningExecutable(t *testing.T) {
	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "govard")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(verify.EnvBinaryOverride, "")

	got := verify.GovardBinaryForTest()
	want, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if got != want {
		t.Fatalf("govardBinary() = %q, want the running executable %q (PATH shim must lose)", got, want)
	}
}

func TestGovardBinaryHonoursOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "govard-custom")
	t.Setenv(verify.EnvBinaryOverride, custom)

	if got := verify.GovardBinaryForTest(); got != custom {
		t.Fatalf("govardBinary() = %q, want the override %q", got, custom)
	}
}
