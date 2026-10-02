package tests

import (
	"errors"
	"os"
	"strings"
	"testing"

	"govard/internal/cmd"
)

const sampleAuthJSON = `{"http-basic":{"repo.example.com":{"username":"u","password":"p"}}}`

func authFiles(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if body, ok := files[path]; ok {
			return []byte(body), nil
		}
		return nil, os.ErrNotExist
	}
}

func noEnv(string) string { return "" }

func TestSandboxServerBuildGetsTheHostComposerAuth(t *testing.T) {
	auth, note := cmd.SandboxComposerAuthForTest(true, true, noEnv, authFiles(map[string]string{"/home/dev/.composer/auth.json": sampleAuthJSON}), "/home/dev", "/work/project")
	if auth != sampleAuthJSON || note != "" {
		t.Fatalf("auth = %q note = %q", auth, note)
	}
}

func TestProjectAuthFileWinsOverTheHomeOne(t *testing.T) {
	project := `{"github-oauth":{"github.com":"token"}}`
	auth, _ := cmd.SandboxComposerAuthForTest(true, true, noEnv, authFiles(map[string]string{
		"/work/project/auth.json":       project,
		"/home/dev/.composer/auth.json": sampleAuthJSON,
	}), "/home/dev", "/work/project")
	if auth != project {
		t.Fatalf("auth = %q, want the project's file first (the order bootstrap uses)", auth)
	}
}

func TestExplicitComposerAuthWins(t *testing.T) {
	getenv := func(key string) string {
		if key == "COMPOSER_AUTH" {
			return `{"explicit":true}`
		}
		return ""
	}
	auth, _ := cmd.SandboxComposerAuthForTest(true, true, getenv, authFiles(map[string]string{"/home/dev/.composer/auth.json": sampleAuthJSON}), "/home/dev", "/work/project")
	if auth != "" {
		t.Fatalf("an explicit COMPOSER_AUTH is already what the executor reads; nothing to add, got %q", auth)
	}
}

func TestRealRemoteNeverGetsHostComposerAuth(t *testing.T) {
	files := authFiles(map[string]string{"/home/dev/.composer/auth.json": sampleAuthJSON})
	if auth, _ := cmd.SandboxComposerAuthForTest(false, true, noEnv, files, "/home/dev", "/work/project"); auth != "" {
		t.Fatalf("a real remote must never receive the developer's credentials, got %q", auth)
	}
}

func TestArtifactDeployReadsNoCredentials(t *testing.T) {
	read := false
	files := func(string) ([]byte, error) { read = true; return []byte(sampleAuthJSON), nil }
	if auth, _ := cmd.SandboxComposerAuthForTest(true, false, noEnv, files, "/home/dev", "/work/project"); auth != "" || read {
		t.Fatalf("an artifact deploy runs no dependency install on the target: auth=%q read=%v", auth, read)
	}
}

func TestInvalidAuthFileIsIgnoredWithANote(t *testing.T) {
	auth, note := cmd.SandboxComposerAuthForTest(true, true, noEnv, authFiles(map[string]string{"/home/dev/.composer/auth.json": "not json"}), "/home/dev", "/work/project")
	if auth != "" {
		t.Fatalf("auth = %q", auth)
	}
	if !strings.Contains(note, "auth.json") || !strings.Contains(note, "valid JSON") {
		t.Fatalf("note = %q, want one naming the file and the problem", note)
	}
	if strings.Contains(note, "not json") {
		t.Fatalf("the note must not echo the file's content: %q", note)
	}
}

func TestUnreadableAuthFileIsSkipped(t *testing.T) {
	files := func(path string) ([]byte, error) { return nil, errors.New("permission denied") }
	if auth, note := cmd.SandboxComposerAuthForTest(true, true, noEnv, files, "/home/dev", "/work/project"); auth != "" || note != "" {
		t.Fatalf("an unreadable file is the same as a missing one: auth=%q note=%q", auth, note)
	}
}

// The credentials belong to the deploy run alone: post-deploy hooks and the
// local commands they start must not inherit them.
func TestComposerAuthIsRemovedAsSoonAsTheRunReturns(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(home+"/.composer", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/.composer/auth.json", []byte(sampleAuthJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("COMPOSER_AUTH", "")
	os.Unsetenv("COMPOSER_AUTH")
	command := cmd.DeployBuildCommand()
	var during string
	cmd.WithSandboxComposerAuthForTest(command, true, true, func() { during = os.Getenv("COMPOSER_AUTH") })
	if !strings.Contains(during, "repo.example.com") {
		t.Fatalf("the run must see the credentials, got %q", during)
	}
	if after, set := os.LookupEnv("COMPOSER_AUTH"); set {
		t.Fatalf("the credentials must be gone once the run returns, got %q", after)
	}
}
