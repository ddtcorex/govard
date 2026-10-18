package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

type recordingPlatform struct {
	mu    sync.Mutex
	emits []recordedEmit
}

type recordedEmit struct {
	event string
	data  any
}

func (p *recordingPlatform) Emit(event string, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emits = append(p.emits, recordedEmit{event: event, data: data})
}

func (p *recordingPlatform) OpenURL(url string) error { return nil }

func (p *recordingPlatform) ChooseDirectory(title, defaultDir string) (string, error) {
	return "", nil
}

func (p *recordingPlatform) ChooseSaveFile(opts desktop.SaveFileOptions) (string, error) {
	return "", nil
}

func (p *recordingPlatform) ChooseOpenFile(title, defaultDir string) (string, error) {
	return "", nil
}

func (p *recordingPlatform) ShowWindow() {}
func (p *recordingPlatform) HideWindow() {}
func (p *recordingPlatform) Quit()       {}

func (p *recordingPlatform) startedPayload() (any, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.emits {
		if e.event == "sync:started" {
			return e.data, true
		}
	}
	return nil, false
}

// The sync:started event must carry the canonical project identity, not the
// identifier the caller happened to use: visual locks and guards keyed on
// the event disagree when one side says the domain and the other the name.
func TestDesktopPkgSyncStartedEventCarriesCanonicalProject(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	projectRoot := t.TempDir()
	baseConfig := strings.TrimSpace(`
project_name: bgsync
framework: laravel
domain: bgsync.test
remotes:
  staging:
    host: stage.example.com
    user: deploy
    path: /var/www/stage
    environment: staging
`) + "\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".govard.yml"), []byte(baseConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        projectRoot,
		ProjectName: "bgsync",
		Domain:      "bgsync.test",
	}); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "govard"), []byte("#!/bin/sh\necho hello-sync\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	platform := &recordingPlatform{}
	err := desktop.RunRemoteSyncBackgroundWithOptionsForTest(
		context.Background(), platform, "bgsync.test", "staging", "files", map[string]bool{},
	)
	if err != nil {
		t.Fatalf("expected background start, got: %v", err)
	}
	defer desktop.ResetSyncingProjectsForTest()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if payload, ok := platform.startedPayload(); ok {
			typed, ok := payload.(desktop.SyncStartedPayload)
			if !ok {
				t.Fatalf("sync:started payload has wrong type %T", payload)
			}
			if typed.Project != "bgsync" {
				t.Fatalf("sync:started project = %q, want canonical %q", typed.Project, "bgsync")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for sync:started")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
