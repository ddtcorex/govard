package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
)

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
