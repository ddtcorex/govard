package tests

import (
	"path/filepath"
	"testing"

	"govard/internal/cmd"
	"govard/internal/desktop"
	"govard/internal/engine"
)

// TestProxyHomeWriterReaderAgreement pins that the engine renderer and both
// compose consumers (svc, desktop) resolve the same proxy directory under a
// GOVARD_HOME_DIR override. Before the Wave C' fix the svc/desktop readers
// built $HOME/.govard/proxy directly while the engine rendered into the
// override, so render and consumer disagreed. Production (override unset)
// resolves both spellings to the same directory, so this is isolation-only.
func TestProxyHomeWriterReaderAgreement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	if got := engine.GovardHomeDir(); got != home {
		t.Fatalf("GovardHomeDir = %q, want %q", got, home)
	}
	want := filepath.Join(home, "proxy")

	if got := cmd.GlobalProxyComposeDirPathForTest(); got != want {
		t.Fatalf("svc proxy dir = %q, want %q", got, want)
	}
	if got := desktop.GlobalServicesComposeDirPathForTest(); got != want {
		t.Fatalf("desktop proxy dir = %q, want %q", got, want)
	}
}
