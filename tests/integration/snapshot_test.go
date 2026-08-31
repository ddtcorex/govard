//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"govard/internal/engine"
)

func TestCreateSnapshot(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-create", files)

	config := engine.Config{
		ProjectName: "snapshot-test",
		Framework:   "magento2",
		Domain:      "snapshot-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	snapshotDir, err := engine.CreateSnapshot(projectDir, config, "test-snapshot")
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		t.Error("Snapshot directory was not created")
	}

	metadataPath := filepath.Join(snapshotDir, "metadata.yml")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Error("Metadata file was not created")
	}
}

func TestCreateSnapshotDuplicateName(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-duplicate", files)

	config := engine.Config{
		ProjectName: "snapshot-dup-test",
		Framework:   "magento2",
		Domain:      "snapshot-dup-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	_, err := engine.CreateSnapshot(projectDir, config, "dup-name")
	if err != nil {
		t.Fatalf("First snapshot creation failed: %v", err)
	}

	_, err = engine.CreateSnapshot(projectDir, config, "dup-name")
	if err == nil {
		t.Error("Expected error for duplicate snapshot name")
	}
}

func TestCreateSnapshotAutoName(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-auto", files)

	config := engine.Config{
		ProjectName: "snapshot-auto-test",
		Framework:   "magento2",
		Domain:      "snapshot-auto-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	snapshotDir, err := engine.CreateSnapshot(projectDir, config, "")
	if err != nil {
		t.Fatalf("Failed to create snapshot with auto name: %v", err)
	}

	if snapshotDir == "" {
		t.Error("Auto-generated snapshot name should not be empty")
	}
}

func TestListSnapshotsEmpty(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-list-empty", files)

	snapshots, err := engine.ListSnapshots(projectDir)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestListSnapshotsWithMetadata(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-list", files)

	config := engine.Config{
		ProjectName: "snapshot-list-test",
		Framework:   "magento2",
		Domain:      "snapshot-list-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	_, err := engine.CreateSnapshot(projectDir, config, "snapshot-1")
	if err != nil {
		t.Fatalf("Failed to create first snapshot: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	_, err = engine.CreateSnapshot(projectDir, config, "snapshot-2")
	if err != nil {
		t.Fatalf("Failed to create second snapshot: %v", err)
	}

	snapshots, err := engine.ListSnapshots(projectDir)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snapshots))
	}

	if snapshots[0].Name != "snapshot-2" {
		t.Error("Snapshots should be sorted by creation date (newest first)")
	}
}

func TestRestoreSnapshotNotFound(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-restore-missing", files)

	config := engine.Config{
		ProjectName: "snapshot-restore-test",
		Framework:   "magento2",
		Domain:      "snapshot-restore-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RestoreSnapshot(projectDir, config, "non-existent", false, false)
	if err == nil {
		t.Error("Expected error for non-existent snapshot")
	}
}

func TestSnapshotMetadataPersistence(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "snapshot-metadata", files)

	config := engine.Config{
		ProjectName: "snapshot-meta-test",
		Framework:   "magento2",
		Domain:      "snapshot-meta-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	snapshotName := "metadata-test"
	_, err := engine.CreateSnapshot(projectDir, config, snapshotName)
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	snapshots, err := engine.ListSnapshots(projectDir)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("Expected 1 snapshot, got %d", len(snapshots))
	}

	meta := snapshots[0]
	if meta.Name != snapshotName {
		t.Errorf("Expected name %s, got %s", snapshotName, meta.Name)
	}
	if meta.Framework != config.Framework {
		t.Errorf("Expected framework %s, got %s", config.Framework, meta.Framework)
	}
	if meta.Domain != config.Domain {
		t.Errorf("Expected domain %s, got %s", config.Domain, meta.Domain)
	}
	if meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSnapshotCommandsWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "snapshot-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	mediaFile := filepath.Join(projectDir, "pub", "media", "catalog", "example.txt")
	if err := os.MkdirAll(filepath.Dir(mediaFile), 0o755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}
	if err := os.WriteFile(mediaFile, []byte("fixture-media"), 0o644); err != nil {
		t.Fatalf("failed to write media fixture: %v", err)
	}

	createResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "create", "test-snapshot")
	createResult.AssertSuccess(t)

	snapshotRoot := filepath.Join(projectDir, ".govard", "snapshots", "test-snapshot")
	if _, err := os.Stat(snapshotRoot); err != nil {
		t.Fatalf("expected snapshot dir to exist: %v", err)
	}

	listResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "list")
	listResult.AssertSuccess(t)
	assertContains(t, listResult.Stdout, "test-snapshot")

	restoreResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "snapshot", "restore", "test-snapshot", "--db-only")
	restoreResult.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|inspect -f {{range .Config.Env}}{{println .}}{{end}} m2-clone-basic-db-1")
	assertContains(t, logs, "docker|exec -i -e MYSQL_PWD=magento m2-clone-basic-db-1 sh -lc")
	assertContains(t, logs, "DUMP_BIN")
	assertContains(t, logs, "mariadb-dump")
	assertContains(t, logs, "DB_CLI")
	assertContains(t, logs, "-u magento magento")
}
