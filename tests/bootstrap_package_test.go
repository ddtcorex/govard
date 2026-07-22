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

func TestBootstrapPkgMagento2FreshCommands(t *testing.T) {
	testCases := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "default version",
			version:  "",
			expected: "composer create-project magento/project-community-edition:2.4.8 .",
		},
		{
			name:     "explicit version",
			version:  "2.4.7",
			expected: "composer create-project magento/project-community-edition:2.4.7 .",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmds := bootstrap.Magento2FreshCommands(bootstrap.Options{Version: tc.version})
			if len(cmds) != 1 {
				t.Fatalf("expected one command, got %d", len(cmds))
			}
			if cmds[0] != tc.expected {
				t.Fatalf("expected command %q, got %q", tc.expected, cmds[0])
			}
		})
	}
}

func TestBootstrapPkgMageOSFreshCommands(t *testing.T) {
	cmds := bootstrap.MageOSFreshCommands(bootstrap.Options{Version: "1.3.1"})
	if len(cmds) != 1 {
		t.Fatalf("expected one command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "mage-os/project-community-edition:1.3.1") || !strings.Contains(cmds[0], "https://repo.mage-os.org") {
		t.Fatalf("unexpected Mage-OS create-project command: %q", cmds[0])
	}
}

func TestBootstrapPkgRunUnsupportedFramework(t *testing.T) {
	err := frameworks.RunBootstrap("unknown", bootstrap.Options{})
	if err == nil {
		t.Fatal("expected unsupported framework error")
	}
	if !strings.Contains(err.Error(), "unsupported framework") {
		t.Fatalf("expected unsupported framework error message, got %v", err)
	}
}
