package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
)

func TestToolExitCodeExtractsExitErrorCode(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 3").Run()
	if err == nil {
		t.Fatal("expected `sh -c exit 3` to return an error")
	}

	code, ok := cmd.ToolExitCodeForTest(err)
	if !ok {
		t.Fatal("expected an *exec.ExitError to be recognized as a tool exit code")
	}
	if code != 3 {
		t.Errorf("ToolExitCodeForTest() code = %d, want 3", code)
	}
}

func TestToolExitCodeRejectsOtherErrors(t *testing.T) {
	if _, ok := cmd.ToolExitCodeForTest(errors.New("docker: command not found")); ok {
		t.Error("expected a non-ExitError to not be treated as a tool exit code")
	}
	if _, ok := cmd.ToolExitCodeForTest(nil); ok {
		t.Error("expected a nil error to not be treated as a tool exit code")
	}
}

func TestFindProjectRootFromFindsNearestGovardYml(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	nested := filepath.Join(root, "app", "code", "Foo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, err := cmd.FindProjectRootFromForTest(nested)
	if err != nil {
		t.Fatalf("FindProjectRootFromForTest(%q) error = %v", nested, err)
	}
	if got != root {
		t.Fatalf("FindProjectRootFromForTest(%q) = %q, want %q", nested, got, root)
	}
}

func TestFindProjectRootFromFailsOutsideProject(t *testing.T) {
	dir := t.TempDir()

	if _, err := cmd.FindProjectRootFromForTest(dir); err == nil {
		t.Fatalf("FindProjectRootFromForTest(%q) expected error, got nil", dir)
	}
}
