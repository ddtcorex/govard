package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestSyncCommandPlan(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
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
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(buf.String(), "rsync") {
		t.Fatalf("expected rsync in plan output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Sync Plan Summary") {
		t.Fatalf("expected summary header in plan output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "planned steps:") {
		t.Fatalf("expected planned steps in plan output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "resume mode: enabled") {
		t.Fatalf("expected resume mode enabled in plan output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "--partial --append-verify") {
		t.Fatalf("expected resume rsync flags in plan output, got: %s", buf.String())
	}
}

func TestSyncCommandPlanIncludeExclude(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
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
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--include", "app/*", "--exclude", "vendor/"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "include patterns: app/*") {
		t.Fatalf("expected include pattern summary, got: %s", out)
	}
	if !strings.Contains(out, "exclude patterns: vendor/") {
		t.Fatalf("expected exclude pattern summary, got: %s", out)
	}
	if !strings.Contains(out, "--include app/*") {
		t.Fatalf("expected include flag in planned rsync command, got: %s", out)
	}
	if !strings.Contains(out, "--exclude vendor/") {
		t.Fatalf("expected exclude flag in planned rsync command, got: %s", out)
	}
}

func TestSyncCommandPlanNoResume(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
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
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--no-resume"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "resume mode: disabled") {
		t.Fatalf("expected resume mode disabled in plan output, got: %s", out)
	}
	if strings.Contains(out, "--append-verify") {
		t.Fatalf("did not expect resume rsync flags when --no-resume is set, got: %s", out)
	}
}

func TestSyncCommandPlanNoCompress(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
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
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--no-compress"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "compression: disabled") {
		t.Fatalf("expected compression disabled in plan output, got: %s", out)
	}
	if strings.Contains(out, "rsync -az") {
		t.Fatalf("did not expect compressed rsync mode when --no-compress is set, got: %s", out)
	}
}

func TestSyncCommandProtectedRemoteDestination(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  prod:
    host: prod.example.com
    user: deploy
    path: /srv/www/app
    protected: true
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
	root.SetArgs([]string{"sync", "--source", "local", "--destination", "prod", "--file"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected protected destination error")
	}
	if !strings.Contains(err.Error(), "write-protected") {
		t.Fatalf("expected protected policy error, got: %v", err)
	}
}

func TestSyncCommandProductionEnvironmentDestinationBlocked(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  production:
    host: prod.example.com
    user: deploy
    path: /srv/www/app
    environment: production
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
	root.SetArgs([]string{"sync", "--source", "local", "--destination", "production", "--file"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected production destination policy error")
	}
	if !strings.Contains(err.Error(), "production environment protection") {
		t.Fatalf("expected production protection error, got: %v", err)
	}
}

func TestSyncCommandRejectsCapabilityMismatch(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
    capabilities:
      files: true
      media: false
      db: true
      deploy: false
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
	root.SetArgs([]string{"sync", "--source", "staging", "--destination", "local", "--media"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected media capability error")
	}
	if !strings.Contains(err.Error(), "does not allow 'media' operations") {
		t.Fatalf("unexpected capability error: %v", err)
	}
}

func TestSyncCommandRejectsRemoteToRemote(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/staging
  production:
    host: prod.example.com
    user: deploy
    path: /srv/www/prod
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
	root.SetArgs([]string{"sync", "--source", "staging", "--destination", "production", "--file"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected local<->remote validation error")
	}
	if !strings.Contains(err.Error(), "local<->remote") {
		t.Fatalf("expected local<->remote error, got: %v", err)
	}
}
