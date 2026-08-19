package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
)

func TestAuditProjectIDSeparatesSameBasenameRepositories(t *testing.T) {
	first := audit.ProjectID("/work/customer-a/shop", "git@example.test:a/shop.git")
	second := audit.ProjectID("/work/customer-b/shop", "git@example.test:b/shop.git")
	if first == second {
		t.Fatal("different canonical roots and repositories produced the same project ID")
	}
}

func TestAuditStoreCreatesSessionAndMonotonicRuns(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	store := newAuditStore(root, now)

	manifest, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     "project-aabbccdd",
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
		BaseRef:       "origin/master",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SessionID != "20260816T120000Z-aabbccdd" {
		t.Fatalf("SessionID = %q", manifest.SessionID)
	}

	first, err := store.CreateRun(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateRun(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID != "run-0001" || second.RunID != "run-0002" {
		t.Fatalf("run IDs = %q, %q", first.RunID, second.RunID)
	}
	if got := store.SessionPath(manifest.ProjectID, manifest.SessionID); got != filepath.Join(root, "project-aabbccdd", "sessions", manifest.SessionID) {
		t.Fatalf("SessionPath = %q", got)
	}
}

func TestAuditStoreRejectsUnsupportedManifestSchemaVersion(t *testing.T) {
	store := newAuditStore(t.TempDir(), time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))

	_, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: 0,
		ProjectID:     "project-aabbccdd",
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
	})
	if err == nil {
		t.Fatal("CreateSession accepted schema version 0")
	}
}

func TestAuditStoreRejectsUnsupportedResultSchemaVersion(t *testing.T) {
	store := newAuditStore(t.TempDir(), time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	manifest, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     "project-aabbccdd",
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	err = store.WriteResult(audit.RunResult{
		SchemaVersion: audit.SchemaVersion + 1,
		ProjectID:     manifest.ProjectID,
		SessionID:     manifest.SessionID,
		RunID:         run.RunID,
		Scope:         audit.ScopeProject,
	})
	if err == nil {
		t.Fatalf("WriteResult accepted schema version %d", audit.SchemaVersion+1)
	}
}

func TestAuditStoreReportsCorruptManifestContextually(t *testing.T) {
	root := t.TempDir()
	store := newAuditStore(root, time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	path := store.SessionPath("project-aabbccdd", "20260816T120000Z-aabbccdd")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "manifest.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.ReadSession("project-aabbccdd", "20260816T120000Z-aabbccdd")
	if err == nil || !strings.Contains(err.Error(), "read session manifest") {
		t.Fatalf("ReadSession error = %v, want contextual manifest error", err)
	}
}

func TestAuditStoreReadSessionRequiresExplicitSession(t *testing.T) {
	root := t.TempDir()
	store := newAuditStore(root, time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	manifest, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     "project-aabbccdd",
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ReadSession(manifest.ProjectID, "")
	if err == nil {
		t.Fatal("ReadSession with no session ID unexpectedly found a session")
	}
}

func TestAuditStoreRoundTripsEmptyResultArrays(t *testing.T) {
	root := t.TempDir()
	store := newAuditStore(root, time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	manifest, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     "project-aabbccdd",
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	result := audit.RunResult{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     manifest.ProjectID,
		SessionID:     manifest.SessionID,
		RunID:         run.RunID,
		Scope:         audit.ScopeProject,
	}
	if err := store.WriteResult(result); err != nil {
		t.Fatal(err)
	}

	stored, err := store.ReadResult(manifest.ProjectID, manifest.SessionID, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Jobs == nil || stored.Artifacts == nil || stored.Errors == nil {
		t.Fatalf("empty arrays = jobs:%#v artifacts:%#v errors:%#v, want non-nil empty slices", stored.Jobs, stored.Artifacts, stored.Errors)
	}
	raw, err := os.ReadFile(filepath.Join(store.SessionPath(manifest.ProjectID, manifest.SessionID), "runs", run.RunID, "audit-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"jobs", "artifacts", "errors"} {
		if string(decoded[key]) != "[]" {
			t.Errorf("serialized %s = %s, want []", key, decoded[key])
		}
	}
}

func TestAuditStoreCleanupOlderThanIsolatedAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	store := newAuditStore(root, time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	old := createAuditSession(t, store, "project-aabbccdd", time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))
	recent := createAuditSession(t, store, "project-aabbccdd", time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
	other := createAuditSession(t, store, "project-deadbeef", time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))

	outside := t.TempDir()
	symlinkPath := filepath.Join(root, "project-aabbccdd", "sessions", "linked-session")
	if err := os.Symlink(outside, symlinkPath); err != nil {
		t.Fatal(err)
	}

	removed, err := store.CleanupOlderThan("project-aabbccdd", time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != old.SessionID {
		t.Fatalf("removed = %q, want only %q", removed, old.SessionID)
	}
	for _, path := range []string{
		store.SessionPath(recent.ProjectID, recent.SessionID),
		store.SessionPath(other.ProjectID, other.SessionID),
		symlinkPath,
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("expected %q to remain: %v", path, err)
		}
	}
}

func newAuditStore(root string, now time.Time) *audit.Store {
	return audit.NewStore(root,
		audit.WithClock(func() time.Time { return now }),
		audit.WithRandomBytes(func(buffer []byte) error {
			copy(buffer, []byte{0xaa, 0xbb, 0xcc, 0xdd})
			return nil
		}),
	)
}

func createAuditSession(t *testing.T, store *audit.Store, projectID string, createdAt time.Time) audit.SessionManifest {
	t.Helper()
	manifest, err := store.CreateSession(audit.SessionManifest{
		SchemaVersion: audit.SchemaVersion,
		ProjectID:     projectID,
		ProjectRoot:   "/work/shop",
		Scope:         audit.ScopeProject,
		CreatedAt:     createdAt,
		SessionID:     createdAt.UTC().Format("20060102T150405Z-") + projectID[len(projectID)-8:],
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
