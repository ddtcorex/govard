package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestRunHooksExecutesCommands(t *testing.T) {
	tempDir := t.TempDir()
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	cfg := engine.Config{
		ProjectName: "demo",
		Domain:      "demo.test",
		Hooks: map[string][]engine.HookStep{
			engine.HookPreUp: {
				{Run: "printf 'ok' > .hook-check"},
			},
		},
	}
	engine.NormalizeConfig(&cfg)

	if err := engine.RunHooks(cfg, engine.HookPreUp, nil, nil); err != nil {
		t.Fatalf("run hooks: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".hook-check"))
	if err != nil {
		t.Fatalf("expected hook artifact: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("expected hook output ok, got %s", string(data))
	}
}

func TestValidateConfigRejectsUnknownHookEvent(t *testing.T) {
	cfg := engine.Config{
		ProjectName: "demo",
		Domain:      "demo.test",
		Hooks: map[string][]engine.HookStep{
			"before_up": {
				{Run: "echo hi"},
			},
		},
	}
	engine.NormalizeConfig(&cfg)

	if err := engine.ValidateConfig(cfg); err == nil {
		t.Fatal("expected validation error for unknown hook event")
	}
}

func TestValidateConfigRejectsEmptyHookCommand(t *testing.T) {
	cfg := engine.Config{
		ProjectName: "demo",
		Domain:      "demo.test",
		Hooks: map[string][]engine.HookStep{
			engine.HookPreUp: {
				{Run: ""},
			},
		},
	}
	engine.NormalizeConfig(&cfg)

	if err := engine.ValidateConfig(cfg); err == nil {
		t.Fatal("expected validation error for empty hook command")
	}
}
