package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// A hung background sync must not hold the syncing flag (and with it every
// mutating environment action) until app restart: the operation is bounded
// by silence, not by wall-clock — a sync that still streams output is
// working, one gone quiet has hung.
func TestBackgroundSyncTimeoutMatchesLongCeiling(t *testing.T) {
	if got := desktop.BackgroundSyncIdleTimeoutForTest(); got != 15*time.Minute {
		t.Fatalf("background sync idle timeout = %v, want 15m", got)
	}
}

func writeSyncGuardProject(t *testing.T, root string) {
	t.Helper()
	content := "project_name: sample-project\nframework: laravel\ndomain: sample-project.test\n" +
		"remotes:\n  staging:\n    host: staging.example.com\n    user: deploy\n    path: /srv/www/staging\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
	// The sync identity resolves through the registry: without an entry
	// the root path canonicalizes to a path key instead of the project
	// name and the flag assertions below cannot observe it.
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")
}

func installGovardShim(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell shim")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "govard")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write govard shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func waitForSyncEvent(t *testing.T, fake *desktop.FakePlatform, event string, timeout time.Duration) []any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := fake.EventsNamed(event); len(got) > 0 {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", event)
	return nil
}

// The cancel for a background sync belongs to the waiter, not the spawner:
// killing the process microseconds after Start breaks every desktop sync.
func TestBackgroundSyncCompletesWithoutInstantCancel(t *testing.T) {
	desktop.ResetStateForTest()
	defer desktop.ResetStateForTest()

	root := t.TempDir()
	writeSyncGuardProject(t, root)
	installGovardShim(t, "sleep 3\necho synced")

	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))
	start := time.Now()
	if msg, err := app.Remote.RunRemoteSync(root, "staging", "files", nil); err != nil {
		t.Fatalf("RunRemoteSync: %v", err)
	} else if msg != "Sync started" {
		t.Fatalf("unexpected message: %q", msg)
	}
	// The flag must be held under the canonical project name while running:
	// asserting only the released state afterwards is vacuous.
	held := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if got := desktop.GetSyncingRemote("sample-project"); got == "staging" {
			held = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !held {
		t.Fatalf("syncing flag never held for sample-project during sync")
	}
	waitForSyncEvent(t, fake, desktop.EventSyncCompleted, 15*time.Second)
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Fatalf("sync finished in %v: the process was killed instead of running", elapsed)
	}
	if failed := fake.EventsNamed(desktop.EventSyncFailed); len(failed) > 0 {
		t.Fatalf("unexpected sync failure: %v", failed)
	}
	if got := desktop.GetSyncingRemote("sample-project"); got != "" {
		t.Fatalf("syncing flag still held by %q after completion", got)
	}
}

// Badge lookups resolve through the canonical project identity: project
// "alpha" sharing its name with syncing project "beta"'s domain must not
// read "syncing" and lock its UI.
func TestSyncingBadgeLookupIsExactForTest(t *testing.T) {
	desktop.ResetStateForTest()
	defer desktop.ResetStateForTest()

	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	for _, entry := range []engine.ProjectRegistryEntry{
		{Path: t.TempDir(), ProjectName: "alpha", Domain: "alpha.test", LastSeenAt: time.Now().UTC()},
		{Path: t.TempDir(), ProjectName: "beta", Domain: "alpha", LastSeenAt: time.Now().UTC()},
	} {
		if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
			t.Fatalf("register project: %v", err)
		}
	}

	desktop.RegisterSyncingProject("beta", "staging")
	defer desktop.UnregisterSyncingProject("beta")

	if got := desktop.GetSyncingRemote("beta"); got != "staging" {
		t.Fatalf("badge lookup missed the syncing project, got %q", got)
	}
	if got := desktop.GetSyncingRemote("BETA"); got != "staging" {
		t.Fatalf("badge lookup should ignore case, got %q", got)
	}
	if got := desktop.GetSyncingRemote("alpha"); got != "" {
		t.Fatalf("unrelated project must not read syncing, got %q", got)
	}
}

// A sync gone quiet past the idle ceiling fails loudly and releases the
// project instead of locking it until app restart.
func TestBackgroundSyncIdleTimeoutReleasesHungSync(t *testing.T) {
	desktop.ResetStateForTest()
	defer desktop.ResetStateForTest()
	restore := desktop.SetBackgroundSyncIdleTimeoutForTest(2 * time.Second)
	defer restore()

	root := t.TempDir()
	writeSyncGuardProject(t, root)
	installGovardShim(t, "sleep 30")

	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))
	if _, err := app.Remote.RunRemoteSync(root, "staging", "files", nil); err != nil {
		t.Fatalf("RunRemoteSync: %v", err)
	}
	if got := desktop.GetSyncingRemote("sample-project"); got != "staging" {
		t.Fatalf("syncing flag not held for sample-project during sync, got %q", got)
	}
	failed := waitForSyncEvent(t, fake, desktop.EventSyncFailed, 20*time.Second)
	matched := false
	for _, payload := range failed {
		if s, ok := payload.(string); ok && strings.Contains(s, "timed out") {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("expected a timed-out failure message, got %v", failed)
	}
	if got := desktop.GetSyncingRemote("sample-project"); got != "" {
		t.Fatalf("syncing flag still held by %q after idle timeout", got)
	}
}
