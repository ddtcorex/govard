package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
)

func TestBootstrapNoCloneDoesNotRequireRemote(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: magento2
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone=false", "--skip-up"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestBootstrapCloneRequiresConfiguredRemote(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: magento2
remotes: {}
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone", "--environment", "dev", "--skip-up"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing remote error")
	}
	if !strings.Contains(err.Error(), "remote 'dev' is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapRejectsFreshAndCloneTogether(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: magento2
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--fresh", "--clone", "--skip-up"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected validation error for --fresh + --clone")
	}
	if !strings.Contains(err.Error(), "--fresh and --clone cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapSupportsSymfony(t *testing.T) {
	config := engine.Config{
		Recipe:      "symfony",
		ProjectName: "symfony-test",
		Domain:      "symfony.test",
	}

	supportedRecipes := []string{"magento2", "symfony", "laravel"}
	found := false
	for _, r := range supportedRecipes {
		if r == config.Recipe {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("symfony should be in supported recipes list")
	}

	opts := bootstrap.DefaultOptions()
	err := bootstrap.Run("symfony", opts)
	if err != nil {
		t.Fatalf("bootstrap.Run(symfony) failed: %v", err)
	}
}

func TestBootstrapRejectsUnsupportedRecipe(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: custom
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone=false", "--skip-up"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported recipe error")
	}
	if !strings.Contains(err.Error(), "bootstrap currently supports") {
		t.Fatalf("unexpected error: %v", err)
	}
}
