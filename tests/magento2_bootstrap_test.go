package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento2"
)

func TestMagento2FreshCommands(t *testing.T) {
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
			cmds := magento2.FreshCommands(bootstrap.Options{Version: tc.version})
			if len(cmds) != 1 {
				t.Fatalf("expected one command, got %d", len(cmds))
			}
			if cmds[0] != tc.expected {
				t.Fatalf("expected command %q, got %q", tc.expected, cmds[0])
			}
		})
	}
}
