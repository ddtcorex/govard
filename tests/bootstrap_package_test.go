package tests

import (
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"
)

func TestBootstrapPkgDefaultOptions(t *testing.T) {
	opts := bootstrap.DefaultOptions()
	if opts.Source != "staging" {
		t.Fatalf("expected source staging, got %s", opts.Source)
	}
}

// Magento2FreshCommands/MageOSFreshCommands moved to
// internal/frameworks/magento2 and internal/frameworks/mageos
// respectively (see tests/magento2_bootstrap_test.go and
// tests/mageos_bootstrap_test.go) - they're no longer part of the
// bootstrap package.

func TestBootstrapPkgRunUnsupportedFramework(t *testing.T) {
	err := frameworks.RunBootstrap("unknown", bootstrap.Options{})
	if err == nil {
		t.Fatal("expected unsupported framework error")
	}
	if !strings.Contains(err.Error(), "unsupported framework") {
		t.Fatalf("expected unsupported framework error message, got %v", err)
	}
}
