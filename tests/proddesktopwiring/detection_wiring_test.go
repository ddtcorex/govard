// Package proddesktopwiring independently verifies that internal/desktop's
// production entrypoint (internal/desktop/app.go) triggers
// internal/frameworks's init()-time registration, without going through
// internal/cmd at all - unlike tests/prodwiring, which imports
// internal/cmd (and therefore, transitively, internal/desktop too, since
// internal/cmd/desktop.go unconditionally imports internal/desktop for the
// `govard desktop` subcommand). That transitive path means
// tests/prodwiring alone cannot distinguish "root.go's own blank import is
// load-bearing" from "app.go's blank import alone is enough" - this
// package isolates the desktop side specifically.
package proddesktopwiring

import (
	"os"
	"path/filepath"
	"testing"

	_ "govard/internal/desktop" // the real desktop entrypoint's import chain, independent of internal/cmd
	"govard/internal/engine"
)

func TestDesktopImportChainRegistersFrameworkDetection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "proddesktopwiring-*")
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
		t.Fatalf("expected framework detection to work via internal/desktop's import chain alone, got %q (want %q) - this means internal/desktop/app.go no longer transitively imports internal/frameworks, so the real govard desktop binary's framework auto-detection is broken", metadata.Framework, "laravel")
	}
}
