package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestEnsureExtensionContractCreatesFiles(t *testing.T) {
	tempDir := t.TempDir()

	changed, err := engine.EnsureExtensionContract(tempDir, false)
	if err != nil {
		t.Fatalf("ensure extension contract: %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("expected scaffold files to be created")
	}

	required := []string{
		".govard/govard.local.yml",
		".govard/docker-compose.override.yml",
		".govard/hooks/pre_up.sh",
		".govard/commands/hello",
	}
	for _, rel := range required {
		path := filepath.Join(tempDir, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected scaffold file %s: %v", rel, err)
		}
	}

	helloPath := filepath.Join(tempDir, ".govard", "commands", "hello")
	info, err := os.Stat(helloPath)
	if err != nil {
		t.Fatalf("stat %s: %v", helloPath, err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("expected %s to be executable", helloPath)
	}

	changed, err = engine.EnsureExtensionContract(tempDir, false)
	if err != nil {
		t.Fatalf("ensure extension contract second run: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("expected no changes on second run, got %v", changed)
	}
}

func TestDiscoverProjectCommands(t *testing.T) {
	tempDir := t.TempDir()
	commandsDir := filepath.Join(tempDir, ".govard", "commands")
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		t.Fatal(err)
	}

	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(commandsDir, name), []byte("#!/usr/bin/env bash\n"), 0755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("deploy.sh")
	write("hello")
	write("bad name.sh")
	write(".ignored")

	commands, err := engine.DiscoverProjectCommands(tempDir)
	if err != nil {
		t.Fatalf("discover project commands: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}
	if commands[0].Name != "deploy" || commands[1].Name != "hello" {
		t.Fatalf("unexpected command names: %+v", commands)
	}
}
