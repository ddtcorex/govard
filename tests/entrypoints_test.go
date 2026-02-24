package tests

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEntrypointGovardVersion(t *testing.T) {
	cmd := exec.Command("go", "run", "../cmd/govard", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run govard version failed: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "v1.3.0") {
		t.Fatalf("expected version output, got %q", string(output))
	}
}

func TestEntrypointDesktopStubs(t *testing.T) {
	for _, target := range []string{"../cmd/govard-desktop", "../desktop"} {
		cmd := exec.Command("go", "run", target)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go run %s failed: %v\n%s", target, err, string(output))
		}
		if !strings.Contains(string(output), "not built yet") {
			t.Fatalf("expected not-built message for %s, got %q", target, string(output))
		}
	}
}
