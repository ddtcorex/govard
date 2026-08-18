package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestSyncPlanDirectoryDetection(t *testing.T) {
	tempDir := t.TempDir()

	sourceDir := filepath.Join(tempDir, "source")
	destDir := filepath.Join(tempDir, "dest")

	if err := os.MkdirAll(filepath.Join(sourceDir, "vendor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	config := engine.Config{
		ProjectName: "test-project",
		Framework:   "magento2",
	}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "staging",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "staging.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: destDir},
	)

	// Test Case 1: --path vendor (no slash) but it exists as a directory
	opts := cmd.SyncExecutionOptionsForTest(true, "", false)
	opts.Path = "vendor"

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the rsync command uses trailing slashes
	foundRsync := false
	for _, cmdStr := range plan.Commands {
		if strings.Contains(cmdStr, "rsync") {
			foundRsync = true
			// Check for trailing slashes in source and destination paths
			// Since both are local in this test, buildRsyncForEndpoints handles them.
			// We should check if the command string includes "vendor/"
			if !strings.Contains(cmdStr, "vendor/") {
				t.Errorf("expected rsync command to contain 'vendor/', got: %s", cmdStr)
			}
		}
	}
	if !foundRsync {
		t.Fatal("rsync command not found in plan")
	}
}

func TestSyncPlanDirectoryDetectionForUnlistedFrameworkPath(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "dest")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	config := engine.Config{ProjectName: "test-project", Framework: "magento2"}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "staging",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "staging.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: destDir},
	)

	// "app/design/frontend/MyTheme" is not in the old hardcoded whitelist and
	// does not exist yet at the destination -- must still be treated as a
	// directory so rsync doesn't nest it inside itself at the destination.
	opts := cmd.SyncExecutionOptionsForTest(true, "", false)
	opts.Path = "app/design/frontend/MyTheme"

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Commands) != 1 {
		t.Fatalf("expected 1 rsync command, got %d", len(plan.Commands))
	}
	if !strings.Contains(plan.Commands[0], "MyTheme/") {
		t.Errorf("expected rsync command to contain trailing-slash 'MyTheme/', got: %s", plan.Commands[0])
	}
}

func TestSyncPlanDirectoryDetectionTreatsExtensionPathAsFile(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "dest")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	config := engine.Config{ProjectName: "test-project", Framework: "magento2"}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "staging",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "staging.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: destDir},
	)

	opts := cmd.SyncExecutionOptionsForTest(true, "", false)
	opts.Path = "app/etc/config.php"

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Commands) != 1 {
		t.Fatalf("expected 1 rsync command, got %d", len(plan.Commands))
	}
	if strings.Contains(plan.Commands[0], "config.php/") {
		t.Errorf("expected file path to NOT get a trailing slash, got: %s", plan.Commands[0])
	}
}

func TestSyncPlanScopes(t *testing.T) {
	// Framework "custom" resolves DB credentials from RemoteConfig directly,
	// without probing the remote over SSH -- keeps this test hermetic.
	config := engine.Config{
		ProjectName: "test-project",
		Framework:   "custom",
	}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "production",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "production.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: "/home/user/project"},
	)

	// Test Case: --full (Files + Media + DB)
	opts := cmd.SyncExecutionOptionsForTest(true, cmd.MediaSyncOptimized, true)

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Verify counts: 2 rsync commands (Files, Media) and 1 DB action
	if len(plan.RsyncCommands) != 2 {
		t.Errorf("expected 2 rsync commands, got %d", len(plan.RsyncCommands))
	}
	if len(plan.RsyncScopes) != 2 {
		t.Errorf("expected 2 rsync scopes, got %d", len(plan.RsyncScopes))
	}
	if len(plan.DatabaseActions) != 1 {
		t.Errorf("expected 1 database action, got %d", len(plan.DatabaseActions))
	}

	// Verify scopes
	if plan.RsyncScopes[0] != cmd.SyncScopeFiles {
		t.Errorf("expected first rsync scope to be %s, got %s", cmd.SyncScopeFiles, plan.RsyncScopes[0])
	}
	if plan.RsyncScopes[1] != cmd.SyncScopeMedia {
		t.Errorf("expected second rsync scope to be %s, got %s", cmd.SyncScopeMedia, plan.RsyncScopes[1])
	}
}

func TestSyncPlanDatabaseUsesTablePrefixForIgnoredTables(t *testing.T) {
	// Exercises the same table-prefix resolution the DB sync action relies on,
	// without going through the remote metadata probe (see BuildDatabaseSyncAction,
	// which now aborts the sync when that probe fails instead of falling back).
	got := cmd.BuildIgnoredTableArgsForTest("magento", "demo_", true, false, "magento2")

	found := false
	for _, arg := range got {
		if arg == "--ignore-table=magento.demo_cron_schedule" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected prefixed ignored table, got: %v", got)
	}
}

// TestSyncPlanDatabaseActionFailsFastWhenLocalContainerNotRunning is a
// regression test: `sync --db` used to build and start the remote dump /
// local import pipeline without ever checking the local DB container was
// up, so against a stopped/missing container it could hang indefinitely
// (docker exec behaving unpredictably) instead of failing with a clear
// error - unlike `db import --stream-db`, which already guarded this. The
// database action must now fail fast, before touching docker exec at all.
func TestSyncPlanDatabaseActionFailsFastWhenLocalContainerNotRunning(t *testing.T) {
	config := engine.Config{
		ProjectName: "test-project",
		Framework:   "custom",
	}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "production",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "production.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: "/home/user/project"},
	)

	opts := cmd.SyncExecutionOptionsForTest(false, "", true)

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatalf("BuildSyncExecutionPlanForTest() error = %v", err)
	}
	if len(plan.DatabaseActions) != 1 {
		t.Fatalf("expected 1 database action, got %d", len(plan.DatabaseActions))
	}

	// No real "test-project-db-1" container exists in this hermetic test
	// environment, so running the action must surface that immediately.
	actionErr := plan.DatabaseActions[0]()
	if actionErr == nil {
		t.Fatal("expected database action to fail when the local container is not running, got nil error")
	}
	if !strings.Contains(actionErr.Error(), "is not running") {
		t.Fatalf("expected a clear 'not running' error, got: %v", actionErr)
	}
}

func TestSyncPlanDatabaseStopsWhenRemoteCredentialsCannotBeResolved(t *testing.T) {
	// Magento2 resolves DB credentials by probing the remote over SSH for app/etc/env.php.
	// Against a host that can't be reached, that probe fails -- the sync must stop
	// with a clear error instead of silently falling back to guessed credentials.
	config := engine.Config{
		ProjectName: "test-project",
		Framework:   "magento2",
	}

	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:     "staging",
			IsLocal:  false,
			RootPath: "/var/www/html",
			RemoteCfg: engine.RemoteConfig{
				Host: "staging.example.com",
				Path: "/var/www/html",
			},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: "/home/user/project"},
	)

	opts := cmd.SyncExecutionOptionsForTest(false, "", true)

	_, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err == nil {
		t.Fatal("expected sync plan to fail when remote DB credentials cannot be resolved, got nil error")
	}
	if !strings.Contains(err.Error(), "cannot sync database") {
		t.Fatalf("expected error to explain the DB sync was stopped, got: %v", err)
	}
}

func TestSyncPlanAdvancedMediaModes(t *testing.T) {
	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{Name: "staging", IsLocal: false, RootPath: "/remote", MediaPath: "/remote/media"},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: "/local", MediaPath: "/local/media"},
	)

	t.Run("Laravel All Mode Includes Cache", func(t *testing.T) {
		config := engine.Config{Framework: "laravel"}
		opts := cmd.SyncExecutionOptionsForTest(false, cmd.MediaSyncAll, false)
		plan, _ := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)

		cmdStr := plan.Commands[0]
		if strings.Contains(cmdStr, "--exclude *cache*/") {
			t.Errorf("expected Laravel 'all' mode to NOT exclude cache, but it did: %s", cmdStr)
		}
	})

	t.Run("Universal Minimal Mode Excludes Images", func(t *testing.T) {
		config := engine.Config{Framework: "wordpress"}
		opts := cmd.SyncExecutionOptionsForTest(false, cmd.MediaSyncMinimal, false)
		plan, _ := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)

		cmdStr := plan.Commands[0]
		if !strings.Contains(cmdStr, "--exclude *.jpg") || !strings.Contains(cmdStr, "--exclude *.png") {
			t.Errorf("expected 'minimal' mode to exclude images, but it didn't: %s", cmdStr)
		}
	})

	t.Run("Media None Mode Skips Sync", func(t *testing.T) {
		config := engine.Config{Framework: "laravel"}
		opts := cmd.SyncExecutionOptionsForTest(false, cmd.MediaSyncNone, false)
		plan, _ := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)

		if len(plan.RsyncCommands) != 0 {
			t.Errorf("expected 0 rsync commands for 'none' mode, got %d", len(plan.RsyncCommands))
		}
	})

	t.Run("WordPress Specific Excludes", func(t *testing.T) {
		config := engine.Config{Framework: "wordpress"}
		opts := cmd.SyncExecutionOptionsForTest(false, cmd.MediaSyncOptimized, false)
		plan, _ := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)

		cmdStr := plan.Commands[0]
		if !strings.Contains(cmdStr, "--exclude *cache*/") {
			t.Errorf("expected WordPress to exclude cache patterns, but it didn't: %s", cmdStr)
		}
	})
}
