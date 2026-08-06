package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/dagster"
)

func TestDagsterBootstrapCapabilities(t *testing.T) {
	b := dagster.NewDagsterBootstrap(bootstrap.Options{})
	if b.Name() != "dagster" {
		t.Errorf("Name() = %q, want %q", b.Name(), "dagster")
	}
	if !b.SupportsFreshInstall() {
		t.Error("expected SupportsFreshInstall() to be true")
	}
	if !b.SupportsClone() {
		t.Error("expected SupportsClone() to be true")
	}
}

func TestDagsterBootstrapPostCloneUsesContainerExecRunner(t *testing.T) {
	var gotContainer, gotScript string
	restore := dagster.SetDagsterContainerExecRunnerForTest(func(containerName, script string) error {
		gotContainer = containerName
		gotScript = script
		return nil
	})
	defer restore()

	b := dagster.NewDagsterBootstrap(bootstrap.Options{ProjectName: "sample-project"})
	if err := b.PostClone(t.TempDir()); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	if gotContainer != "sample-project-web-1" {
		t.Errorf("containerName = %q, want %q", gotContainer, "sample-project-web-1")
	}
	wantScript := "pip install --no-cache-dir -r requirements.txt; rc=$?; chown -R \"$(stat -c %u:%g .)\" . 2>/dev/null; exit $rc"
	if gotScript != wantScript {
		t.Errorf("script = %q, want %q", gotScript, wantScript)
	}
}

func TestWriteDagsterConfigFilesWritesPostgresStorageConfig(t *testing.T) {
	projectDir := t.TempDir()
	pyproject := "[project]\nname = \"my-pipeline\"\n\n[tool.dagster]\nmodule_name = \"my_pipeline.definitions\"\ncode_location_name = \"my_pipeline\"\n"
	if err := os.WriteFile(filepath.Join(projectDir, "pyproject.toml"), []byte(pyproject), 0o644); err != nil {
		t.Fatalf("write pyproject.toml fixture: %v", err)
	}

	if err := dagster.WriteDagsterConfigFilesForTest(projectDir); err != nil {
		t.Fatalf("WriteDagsterConfigFilesForTest() error = %v", err)
	}

	dagsterYAML, err := os.ReadFile(filepath.Join(projectDir, "dagster.yaml"))
	if err != nil {
		t.Fatalf("read dagster.yaml: %v", err)
	}
	content := string(dagsterYAML)
	for _, want := range []string{
		"storage:", "postgres:", "postgres_db:",
		"env: POSTGRES_USER", "env: POSTGRES_PASSWORD",
		"env: POSTGRES_HOST", "env: POSTGRES_DB", "env: POSTGRES_PORT",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("dagster.yaml missing %q, got:\n%s", want, content)
		}
	}

	workspaceYAML, err := os.ReadFile(filepath.Join(projectDir, "workspace.yaml"))
	if err != nil {
		t.Fatalf("read workspace.yaml: %v", err)
	}
	wantWorkspace := "load_from:\n  - python_module: my_pipeline.definitions\n"
	if string(workspaceYAML) != wantWorkspace {
		t.Errorf("workspace.yaml = %q, want %q", string(workspaceYAML), wantWorkspace)
	}
}

func TestWriteDagsterConfigFilesErrorsWithoutPyprojectToml(t *testing.T) {
	projectDir := t.TempDir()
	if err := dagster.WriteDagsterConfigFilesForTest(projectDir); err == nil {
		t.Fatal("expected error when pyproject.toml is missing, got nil")
	}
}

func TestModuleNameFromPyprojectToml(t *testing.T) {
	content := "[project]\nname = \"my-pipeline\"\n\n[tool.dagster]\nmodule_name = \"my_pipeline.definitions\"\ncode_location_name = \"my_pipeline\"\n"
	got, err := dagster.ModuleNameFromPyprojectTomlForTest(content)
	if err != nil {
		t.Fatalf("ModuleNameFromPyprojectTomlForTest() error = %v", err)
	}
	if got != "my_pipeline.definitions" {
		t.Errorf("ModuleNameFromPyprojectTomlForTest() = %q, want %q", got, "my_pipeline.definitions")
	}
}

func TestModuleNameFromPyprojectTomlErrorsWithoutToolDagsterSection(t *testing.T) {
	content := "[project]\nname = \"my-pipeline\"\n"
	if _, err := dagster.ModuleNameFromPyprojectTomlForTest(content); err == nil {
		t.Fatal("expected error when [tool.dagster] section is missing, got nil")
	}
}
