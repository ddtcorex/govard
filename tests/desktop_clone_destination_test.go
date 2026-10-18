package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// Wiping the clone destination deletes every entry in it, so hidden
// directories (.config, .ssh and friends) are refused like the home
// directory is: nobody onboards a project into one on purpose.
func TestDesktopPkgValidateSafeCloneDestinationRefusesHiddenDirs(t *testing.T) {
	hidden := filepath.Join(t.TempDir(), ".config")
	if err := desktop.ValidateSafeCloneDestinationForTest(hidden); err == nil {
		t.Fatalf("expected hidden directory %q to be refused", hidden)
	} else if !strings.Contains(strings.ToLower(err.Error()), "hidden") {
		t.Fatalf("expected a hidden-directory refusal, got: %v", err)
	}

	plain := filepath.Join(t.TempDir(), "shop")
	if err := desktop.ValidateSafeCloneDestinationForTest(plain); err != nil {
		t.Fatalf("expected a plain directory to pass, got: %v", err)
	}
}
