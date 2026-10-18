package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

// Delete output is invisible in the desktop app (no terminal), so warnings
// the engine prints during deletion must come back in the result message;
// plain progress chatter must not.
func TestDesktopPkgAppendDeleteWarnings(t *testing.T) {
	output := "Stopping containers...\nWarning: pre-delete hooks failed: boom\nRemoved network foo\nWarning: docker compose down -v: nope\n"
	message := desktop.AppendDeleteWarningsForTest("Project deleted successfully", output)
	if !strings.Contains(message, "pre-delete hooks failed: boom") {
		t.Fatalf("hook warning lost: %q", message)
	}
	if !strings.Contains(message, "docker compose down -v: nope") {
		t.Fatalf("docker warning lost: %q", message)
	}
	if strings.Contains(message, "Removed network foo") {
		t.Fatalf("progress chatter leaked into message: %q", message)
	}
	if !strings.HasPrefix(message, "Project deleted successfully") {
		t.Fatalf("base message must lead: %q", message)
	}

	plain := desktop.AppendDeleteWarningsForTest("Project deleted successfully", "Stopping...\nDone.\n")
	if plain != "Project deleted successfully" {
		t.Fatalf("warning-free output must leave the message alone, got %q", plain)
	}
}
