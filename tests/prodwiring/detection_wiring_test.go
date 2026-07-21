// Package prodwiring verifies that Govard's real CLI entrypoint
// (internal/cmd's import chain, which is what cmd/govard/main.go actually
// imports) triggers internal/frameworks's init()-time registration - not
// just the tests package, which already imports internal/frameworks
// directly elsewhere and would mask a missing production import.
//
// Note: internal/cmd unconditionally imports internal/desktop too (via
// internal/cmd/desktop.go, for the `govard desktop` subcommand), so this
// test alone cannot distinguish "internal/cmd/root.go's own blank import
// is load-bearing" from "internal/desktop/app.go's blank import alone is
// enough, reached transitively through cmd's dependency on desktop." It
// proves the CLI binary's overall dependency graph registers detection
// data - which is what actually matters for correctness - and would only
// fail if BOTH blank imports were removed. See
// tests/proddesktopwiring for a test that independently isolates the
// desktop side by importing internal/desktop directly, without going
// through internal/cmd at all.
package prodwiring

import (
	"os"
	"path/filepath"
	"testing"

	_ "govard/internal/cmd" // the real CLI entrypoint's import chain
	"govard/internal/engine"
)

func TestCLIImportChainRegistersFrameworkDetection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prodwiring-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	composerJSON := `{"require":{"laravel/framework":"11.0.0"}}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatalf("failed to write composer.json: %v", err)
	}

	metadata := engine.DetectFramework(tempDir)
	if metadata.Framework != "laravel" {
		t.Fatalf("expected framework detection to work via internal/cmd's import chain, got %q (want %q) - this means internal/cmd no longer transitively imports internal/frameworks, so the real govard CLI binary's framework auto-detection is broken", metadata.Framework, "laravel")
	}
}
