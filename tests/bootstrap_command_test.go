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

	"github.com/spf13/cobra"
)

func TestBootstrapNoCloneDoesNotRequireRemote(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	config := `project_name: test
domain: test.test
framework: magento2
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	cmd.ResetBootstrapFlags()
	root := cmd.RootCommandForTest()

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone=false", "--skip-up", "--no-db", "--no-media", "--no-composer"})

	// IMPORTANT: Mock the subcommand runner to prevent recursion/fork-bomb during tests
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		return nil
	})()

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestBootstrapCloneRequiresConfiguredRemote(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	config := `project_name: test
domain: test.test
framework: magento2
remotes: {}
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	cmd.ResetBootstrapFlags()
	root := cmd.RootCommandForTest()

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone", "--environment", "dev", "--skip-up"})

	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		return nil
	})()

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
	configPath := filepath.Join(tempDir, ".govard.yml")
	config := `project_name: test
domain: test.test
framework: magento2
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	cmd.ResetBootstrapFlags()
	root := cmd.RootCommandForTest()

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--fresh", "--clone", "--skip-up"})

	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		return nil
	})()

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
		Framework:   "symfony",
		ProjectName: "symfony-test",
		Domain:      "symfony.test",
	}

	supportedFrameworks := []string{"magento2", "symfony", "laravel"}
	found := false
	for _, r := range supportedFrameworks {
		if r == config.Framework {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("symfony should be in supported frameworks list")
	}

	opts := bootstrap.DefaultOptions()
	err := bootstrap.Run("symfony", opts)
	if err != nil {
		t.Fatalf("bootstrap.Run(symfony) failed: %v", err)
	}
}

func TestBootstrapRejectsUnsupportedFramework(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	config := `project_name: test
domain: test.test
framework: custom
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	cmd.ResetBootstrapFlags()
	root := cmd.RootCommandForTest()

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--clone=false", "--skip-up"})

	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		return nil
	})()

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported framework error")
	}
	if !strings.Contains(err.Error(), "bootstrap currently supports") {
		t.Fatalf("unexpected error: %v", err)
	}
}
