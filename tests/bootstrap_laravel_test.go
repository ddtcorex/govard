package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/laravel"
)

func TestBootstrapPkgLaravelFreshCommands(t *testing.T) {
	cases := []struct {
		version  string
		expected string
	}{
		{"12", "laravel/laravel:^12.0"},
		{"11", "laravel/laravel:^11.0"},
		{"10", "laravel/laravel:^10.0"},
		{"9", "laravel/laravel:^9.0"},
		{"", "laravel/laravel:^11.0"},
	}

	for _, tc := range cases {
		opts := bootstrap.Options{Version: tc.version}
		laravel := laravel.NewLaravelBootstrap(opts)
		cmds := laravel.FreshCommands()

		if len(cmds) == 0 {
			t.Fatalf("expected commands for version %s, got none", tc.version)
		}

		if !containsSubstring(cmds[0], tc.expected) {
			t.Errorf("expected command to contain %q for version %s, got %q", tc.expected, tc.version, cmds[0])
		}
	}
}

func TestLaravelCreateProjectWithRunnerStagesComposerCreateProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var capturedCommand string
	laravelBootstrap := laravel.NewLaravelBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			capturedCommand = command
			stageDir := extractStageHostDir(t, command)
			return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"laravel-app\"}\n"), 0o644)
		},
	})

	if err := laravelBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if !strings.Contains(capturedCommand, `composer create-project laravel/laravel:^11.0 "$GOVARD_STAGE_DIR" --no-interaction`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
}
