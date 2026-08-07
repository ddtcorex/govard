package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/nextjs"
)

func TestBootstrapPkgNextJSFreshInstallSupport(t *testing.T) {
	opts := bootstrap.Options{}
	nextjsBootstrap := nextjs.NewNextJSBootstrap(opts)

	if !nextjsBootstrap.SupportsFreshInstall() {
		t.Error("expected Next.js to support fresh install")
	}

	if !nextjsBootstrap.SupportsClone() {
		t.Error("expected Next.js to support clone")
	}
}

func TestNextJSCreateProjectStagesIntoTemporaryDirectory(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write stale.txt: %v", err)
	}

	var stageDir string
	restore := nextjs.SetNextJSStageProjectCreatorForTest(func(dir string) error {
		stageDir = dir
		return os.WriteFile(filepath.Join(dir, "package.json"), []byte("{\"name\":\"next-app\"}\n"), 0o644)
	})
	defer restore()

	nextJSBootstrap := nextjs.NewNextJSBootstrap(bootstrap.Options{})
	if err := nextJSBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if stageDir == "" {
		t.Fatal("expected staged directory to be captured")
	}
	if stageDir == projectDir {
		t.Fatalf("expected staged directory to differ from project dir")
	}
	if filepath.Dir(stageDir) != projectDir {
		t.Fatalf("expected staged directory to live under project dir, got %s", stageDir)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.txt to be removed, got err=%v", err)
	}
}

func TestNextJSCreateProjectWithRunnerStagesCreateNextApp(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var capturedCommand string
	nextJSBootstrap := nextjs.NewNextJSBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			capturedCommand = command
			stageDir := extractStageHostDir(t, command)
			return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"nextjs-app\"}\n"), 0o644)
		},
	})

	if err := nextJSBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if !strings.Contains(capturedCommand, `npx create-next-app@latest "$GOVARD_STAGE_DIR" --typescript --tailwind --eslint --app --no-src-dir --import-alias '@/*' --use-npm --yes`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if !strings.Contains(capturedCommand, "GOVARD_STAGE_DIR='/app/") {
		t.Fatalf("expected staged dir under NodeWorkDir (/app), got: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied into project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
}
