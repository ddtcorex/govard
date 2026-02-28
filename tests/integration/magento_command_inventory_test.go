//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
)

func TestMagentoRelevantCommandsPresent(t *testing.T) {
	env := NewTestEnvironment(t)

	result := env.RunGovard(t, t.TempDir(), "--help")
	result.AssertSuccess(t)

	required := []string{
		"sync",
		"bootstrap",
		"db",
		"config",
		"tool",
		"env",
		"deploy",
		"remote",
		"snapshot",
		"upgrade",
	}

	for _, name := range required {
		assertContains(t, result.Stdout, name)
	}

	if strings.Contains(result.Stdout, "\n  deps") {
		t.Fatalf("expected root help to exclude deps command, got:\n%s", result.Stdout)
	}
}
