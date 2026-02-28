package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestLockCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()

	lockCommand, _, err := root.Find([]string{"lock"})
	if err != nil {
		t.Fatalf("find lock command: %v", err)
	}
	if lockCommand == nil || lockCommand.Use != "lock" {
		t.Fatalf("unexpected lock command: %#v", lockCommand)
	}

	generateCommand, _, err := root.Find([]string{"lock", "generate"})
	if err != nil {
		t.Fatalf("find lock generate command: %v", err)
	}
	if generateCommand == nil || generateCommand.Use != "generate" {
		t.Fatalf("unexpected lock generate command: %#v", generateCommand)
	}

	checkCommand, _, err := root.Find([]string{"lock", "check"})
	if err != nil {
		t.Fatalf("find lock check command: %v", err)
	}
	if checkCommand == nil || checkCommand.Use != "check" {
		t.Fatalf("unexpected lock check command: %#v", checkCommand)
	}
}

func TestLockGenerateCommandWritesLockFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: magento2
`), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetLockDependenciesForTest(engine.LockDependencies{
		ReadDockerVersion:        func() (string, error) { return "27.2.1", nil },
		ReadDockerComposeVersion: func() (string, error) { return "2.29.7", nil },
		ReadServiceImages: func(composePath string) (map[string]string, error) {
			_ = composePath
			return map[string]string{"web": "nginx:1.27"}, nil
		},
		Now: func() time.Time { return time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC) },
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"lock", "generate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute lock generate: %v", err)
	}

	lock, err := engine.ReadLockFile(filepath.Join(tempDir, "govard.lock"))
	if err != nil {
		t.Fatalf("read generated lockfile: %v", err)
	}
	if lock.Host.DockerVersion != "27.2.1" {
		t.Fatalf("expected docker version 27.2.1, got %s", lock.Host.DockerVersion)
	}
	if lock.Services["web"] != "nginx:1.27" {
		t.Fatalf("expected web image nginx:1.27, got %s", lock.Services["web"])
	}
}

func TestLockCheckCommandReturnsErrorOnMismatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: magento2
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.WriteLockFile(filepath.Join(tempDir, "govard.lock"), engine.LockFile{
		Version:     1,
		GeneratedAt: "2026-02-20T12:00:00Z",
		Govard: engine.LockGovardInfo{
			Version: "0.9.0",
		},
		Host: engine.LockHostInfo{
			OS:                   "linux",
			Arch:                 "amd64",
			DockerVersion:        "27.2.1",
			DockerComposeVersion: "2.29.7",
		},
		Project: engine.LockProjectInfo{Name: "demo", Framework: "magento2"},
	}); err != nil {
		t.Fatalf("write lock file fixture: %v", err)
	}

	restore := cmd.SetLockDependenciesForTest(engine.LockDependencies{
		ReadDockerVersion:        func() (string, error) { return "27.2.1", nil },
		ReadDockerComposeVersion: func() (string, error) { return "2.29.7", nil },
		ReadServiceImages:        func(composePath string) (map[string]string, error) { return map[string]string{}, nil },
		Now:                      func() time.Time { return time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC) },
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	buf := &strings.Builder{}
	root := cmd.RootCommandForTest()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"lock", "check"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected lock check mismatch error")
	}
	if !strings.Contains(strings.ToLower(buf.String()), "mismatch") {
		t.Fatalf("expected mismatch output, got: %s", buf.String())
	}
}

func TestBuildUpLockWarningsForTest(t *testing.T) {
	expected := engine.LockFile{
		Govard: engine.LockGovardInfo{Version: "1.0.0"},
		Host:   engine.LockHostInfo{DockerVersion: "27.2.1", DockerComposeVersion: "2.29.7"},
		Project: engine.LockProjectInfo{
			Name:      "demo",
			Framework: "magento2",
		},
	}
	current := expected
	current.Host.DockerComposeVersion = "2.30.0"

	warnings := cmd.BuildUpLockWarningsForTest(expected, current)
	if len(warnings) == 0 {
		t.Fatal("expected warnings for lock mismatch")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "docker_compose_version") {
		t.Fatalf("expected docker compose mismatch warning, got: %s", joined)
	}
}
