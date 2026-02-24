package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
)

func TestExtensionsCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()

	command, _, err := root.Find([]string{"extensions"})
	if err != nil {
		t.Fatalf("find extensions: %v", err)
	}
	if command == nil || command.Use != "extensions" {
		t.Fatalf("unexpected extensions command: %+v", command)
	}

	command, _, err = root.Find([]string{"custom"})
	if err != nil {
		t.Fatalf("find custom: %v", err)
	}
	if command == nil || command.Use != "custom" {
		t.Fatalf("unexpected custom command: %+v", command)
	}
}

func TestExtensionsInitCreatesContract(t *testing.T) {
	tempDir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"extensions", "init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute extensions init: %v", err)
	}

	required := []string{
		".govard/govard.local.yml",
		".govard/docker-compose.override.yml",
		".govard/hooks/pre_up.sh",
		".govard/commands/hello",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(tempDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing scaffold file %s: %v", rel, err)
		}
	}
}
