package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestRunStagedCreateProjectForTestPreservesGovardFiles(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".govard"), 0o755); err != nil {
		t.Fatalf("mkdir .govard: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard", "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("write .govard/keep.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write stale.txt: %v", err)
	}

	err := bootstrap.RunStagedCreateProjectForTest(projectDir, nil, func(stageDir string) error {
		if err := os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"sample\"}\n"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(stageDir, "src"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stageDir, "src", "main.js"), []byte("console.log('ok')\n"), 0o644)
	}, "")
	if err != nil {
		t.Fatalf("RunStagedCreateProjectForTest() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".govard", "keep.txt")); err != nil {
		t.Fatalf("expected .govard contents to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "src", "main.js")); err != nil {
		t.Fatalf("expected staged src/main.js to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.txt to be removed, got err=%v", err)
	}
}

func extractStageHostDir(t *testing.T, command string) string {
	t.Helper()
	match := regexp.MustCompile(`GOVARD_STAGE_HOST_DIR='([^']+)'`).FindStringSubmatch(command)
	if len(match) != 2 {
		t.Fatalf("could not extract GOVARD_STAGE_HOST_DIR from command: %s", command)
	}
	return match[1]
}
