package tests

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
)

func TestEnsureRemoteKnownResolvesSandboxWhenRunning(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{Host: "127.0.0.1", User: deploy.SandboxUser, Sandbox: true}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	name, remote, err := cmd.EnsureRemoteKnownForTest(engine.Config{ProjectName: "sample-project"}, "sandbox")
	if err != nil {
		t.Fatalf("ensureRemoteKnown: %v", err)
	}
	if name != "sandbox" || remote.Host != "127.0.0.1" {
		t.Fatalf("name=%q remote=%+v, want the synthetic sandbox remote", name, remote)
	}
}

func TestEnsureRemoteKnownSurfacesTheDormantError(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessDormant, fmt.Errorf("sandbox container govard-sample-sandbox-abc123 is not running; run 'govard sandbox up' to start it")
	})
	defer restore()

	_, _, err := cmd.EnsureRemoteKnownForTest(engine.Config{ProjectName: "sample-project"}, "sandbox")
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("err = %v, want the dormant message propagated unchanged", err)
	}
}

func TestRemoteAddRefusesTheReservedSandboxName(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: sample-project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "sandbox", "--host", "127.0.0.1", "--user", "deployer", "--path", "/tmp"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "reserved for the implicit sandbox remote") {
		t.Fatalf("err = %v, want the reserved-name refusal", err)
	}
}

func TestRemoteListRendersConfiguredAndSandboxRows(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  staging:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "staging") {
		t.Fatalf("output missing the configured remote:\n%s", text)
	}
	if !strings.Contains(text, "sandbox") || !strings.Contains(text, "implicit") || !strings.Contains(text, "absent") {
		t.Fatalf("output missing the synthetic sandbox row:\n%s", text)
	}
}

func TestRemoteListDoesNotDuplicateSandboxRow(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  staging:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
  sandbox:
    host: legacy.example.com
    user: deploy
    path: /srv/legacy
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	rows := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(strings.ToLower(line), "sandbox") {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("expected exactly one sandbox row, got %d:\n%s", rows, out.String())
	}
}

func TestShadowRemotesSandboxWarns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  sandbox:
    host: legacy.example.com
    user: deploy
    path: /srv/legacy
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	if !strings.Contains(errOut.String(), "shadow") {
		t.Fatalf("a user-defined remotes.sandbox must warn that the synthetic sandbox shadows it.\nstdout:\n%s\nstderr:\n%s", out.String(), errOut.String())
	}
}
