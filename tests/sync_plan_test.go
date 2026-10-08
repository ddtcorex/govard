package tests

import (
	"encoding/base64"
	"encoding/json"
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

// installSyncPasswordFakes puts a fake ssh and a fake docker on PATH. ssh
// answers the credential probe (the remote command contains "php -r") with
// payload and records every other invocation; docker reports the local DB
// container as running, records the rest and drains stdin. Each recorded
// invocation is one line in the returned log file.
func installSyncPasswordFakes(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	ssh := "#!/bin/sh\ncase \"$*\" in\n*'php -r'*) printf '%s' \"$FAKE_SSH_PAYLOAD\"; exit 0;;\nesac\nprintf 'SSH %s\\n' \"$*\" >> \"$FAKE_CALL_LOG\"\nexit 0\n"
	docker := "#!/bin/sh\ncase \"$1\" in\ninspect) echo true; exit 0;;\nesac\nprintf 'DOCKER %s\\n' \"$*\" >> \"$FAKE_CALL_LOG\"\ncat >/dev/null\nexit 0\n"
	for name, body := range map[string]string{"ssh": ssh, "docker": docker} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_SSH_PAYLOAD", payload)
	t.Setenv("FAKE_CALL_LOG", log)
	return log
}

// TestSyncPlanNeverContainsTheDBPassword pins #514: the plan shown by
// `sync --plan` and the interactive confirmation must never carry the remote
// database password (probed or configured), while the command the action
// executes still carries the exact value.
func TestSyncPlanNeverContainsTheDBPassword(t *testing.T) {
	passwords := []string{"p'w$x", "SECRETPW", "pw*$", "line1\nline2", "***"}
	cases := []struct {
		name      string
		framework string
		remoteCfg func(password string) engine.RemoteConfig
		pull      bool
		// envVar is the password variable the framework's DB engine exports:
		// MYSQL_PWD by default, PGPASSWORD for a Postgres framework.
		envVar string
	}{
		{"magento2 probe pull", "magento2", func(string) engine.RemoteConfig { return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www"} }, true, "MYSQL_PWD"},
		{"laravel probe pull", "laravel", func(string) engine.RemoteConfig { return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www"} }, true, "MYSQL_PWD"},
		{"magento2 probe push", "magento2", func(string) engine.RemoteConfig { return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www"} }, false, "MYSQL_PWD"},
		{"custom db_pass pull", "custom", func(pw string) engine.RemoteConfig {
			return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www", DBName: "app", DBUser: "app", DBPass: pw}
		}, true, "MYSQL_PWD"},
		{"custom db_pass push", "custom", func(pw string) engine.RemoteConfig {
			return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www", DBName: "app", DBUser: "app", DBPass: pw}
		}, false, "MYSQL_PWD"},
		{"postgres django db_pass pull", "django", func(pw string) engine.RemoteConfig {
			return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www", DBName: "app", DBUser: "app", DBPass: pw}
		}, true, "PGPASSWORD"},
		{"postgres django db_pass push", "django", func(pw string) engine.RemoteConfig {
			return engine.RemoteConfig{Host: "r.example.com", Path: "/var/www", DBName: "app", DBUser: "app", DBPass: pw}
		}, false, "PGPASSWORD"},
	}

	for _, password := range passwords {
		for _, tc := range cases {
			t.Run(tc.name+" "+password, func(t *testing.T) {
				raw, err := json.Marshal(map[string]string{
					"host": "127.0.0.1", "username": "app", "dbname": "app", "password": password,
					"db_username": "app", "db_database": "app", "db_password": password,
				})
				if err != nil {
					t.Fatalf("marshal payload: %v", err)
				}
				log := installSyncPasswordFakes(t, base64.StdEncoding.EncodeToString(raw))

				remoteEndpoint := cmd.SyncEndpoint{Name: "staging", RootPath: "/var/www", RemoteCfg: tc.remoteCfg(password)}
				localEndpoint := cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: t.TempDir()}
				source, destination := remoteEndpoint, localEndpoint
				if !tc.pull {
					source, destination = localEndpoint, remoteEndpoint
				}
				endpoints := cmd.ResolveSyncEndpointsForTest(source, destination)
				opts := cmd.SyncExecutionOptionsForTest(false, "", true)
				config := engine.Config{ProjectName: "test-project", Framework: tc.framework}

				plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
				if err != nil {
					t.Fatalf("BuildSyncExecutionPlanForTest() error = %v", err)
				}

				lines := append([]string{}, plan.Descriptions...)
				lines = append(lines, plan.Commands...)
				lines = append(lines, cmd.BuildSyncPlanSummaryForTest(endpoints, plan, opts)...)
				shown := strings.Join(lines, "\n")
				// "***" is itself the placeholder, so for that password only the
				// shell-quoted form (which never appears in a redacted plan) is checkable.
				if (password != "***" && strings.Contains(shown, password)) || strings.Contains(shown, engine.ShellQuote(password)) {
					t.Fatalf("plan output leaks the password %q:\n%s", password, shown)
				}
				if !strings.Contains(shown, tc.envVar+"=***;") {
					t.Fatalf("plan output should show the redacted %s=*** placeholder:\n%s", tc.envVar, shown)
				}

				if len(plan.DatabaseActions) != 1 {
					t.Fatalf("expected 1 database action, got %d", len(plan.DatabaseActions))
				}
				// The stream itself is empty, so the action's own result is not
				// the assertion; what matters is the command it executed.
				_ = plan.DatabaseActions[0]()
				calls, err := os.ReadFile(log)
				if err != nil {
					t.Fatalf("the action never ran a command: %v", err)
				}
				want := "export " + tc.envVar + "=" + engine.ShellQuote(password) + ";"
				if !strings.Contains(string(calls), want) {
					t.Fatalf("executed command must carry the real password %q, calls:\n%s", want, calls)
				}
			})
		}
	}
}

func TestSyncPlanDegradesWhenRemoteDBProbeFails(t *testing.T) {
	config := engine.Config{ProjectName: "test-project", Framework: "laravel"}
	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:      "staging",
			IsLocal:   false,
			RootPath:  "/var/www/html",
			RemoteCfg: engine.RemoteConfig{Host: "127.0.0.1", Port: 1, User: "nobody", Path: "/var/www/html"},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: t.TempDir()},
	)

	opts := cmd.SyncExecutionOptionsForTest(false, "", true)
	if _, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts); err == nil {
		t.Fatal("a real sync must still refuse to run without remote DB credentials")
	}

	opts.PlanOnly = true
	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatalf("--plan must degrade instead of failing: %v", err)
	}
	if len(plan.Commands) != 1 || !strings.Contains(plan.Commands[0], "could not be probed") {
		t.Fatalf("plan must say the remote credentials were not probed, got %v", plan.Commands)
	}
	if len(plan.DatabaseActions) != 1 {
		t.Fatalf("expected a database action, got %d", len(plan.DatabaseActions))
	}
}

func TestSyncPlanOmitsUnresolvablePrivacyFilterWhenPrefixUnknown(t *testing.T) {
	config := engine.Config{ProjectName: "test-project", Framework: "wordpress"}
	endpoints := cmd.ResolveSyncEndpointsForTest(
		cmd.SyncEndpoint{
			Name:      "staging",
			IsLocal:   false,
			RootPath:  "/var/www/html",
			RemoteCfg: engine.RemoteConfig{Host: "127.0.0.1", Port: 1, User: "nobody", Path: "/var/www/html"},
		},
		cmd.SyncEndpoint{Name: "local", IsLocal: true, RootPath: t.TempDir()},
	)
	opts := cmd.SyncExecutionOptionsForTest(false, "", true)
	opts.PlanOnly = true
	opts.NoPII = true
	opts.NoNoise = true

	plan, err := cmd.BuildSyncExecutionPlanForTest(config, endpoints, opts)
	if err != nil {
		t.Fatalf("--plan must not fail: %v", err)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("expected one DB command, got %v", plan.Commands)
	}
	shown := plan.Commands[0]
	if strings.Contains(shown, "--ignore-table") {
		t.Fatalf("plan must not show unprefixed --ignore-table names that cannot match:\n%s", shown)
	}
	for _, want := range []string{"privacy filter is unresolved", "would be refused"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("plan must mention %q:\n%s", want, shown)
		}
	}
}
