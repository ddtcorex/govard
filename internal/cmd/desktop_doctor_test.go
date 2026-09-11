package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDesktopVersionProbeIsBounded covers the reason `desktop doctor` can hang:
// a desktop build that does not understand --version starts its GUI instead and
// never returns. The probe must give up on the caller's deadline, so the
// diagnostic still reports something.
func TestDesktopVersionProbeIsBounded(t *testing.T) {
	script := filepath.Join(t.TempDir(), "govard-desktop")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake desktop binary: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := probeDesktopVersion(ctx, script)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("probeDesktopVersion = nil, want a deadline error")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("probe took %s; it must stop at the caller's deadline", elapsed)
	}
}
