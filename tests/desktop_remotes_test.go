package tests

import (
	"reflect"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func TestDesktopPkgBuildRemoteEntriesForTest(t *testing.T) {
	entries := desktop.BuildRemoteEntriesForTest(map[string]desktop.RemoteConfigSnapshot{
		"staging": {
			Host:       "staging.example.com",
			User:       "deploy",
			Path:       "/var/www/staging",
			Port:       2222,
			AuthMethod: "ssh-agent",
			Capabilities: []string{
				"files",
				"media",
				"db",
			},
		},
		"prod": {
			Host:       "prod.example.com",
			User:       "root",
			Path:       "/srv/www/prod",
			Port:       2222,
			AuthMethod: "ssh-agent",
			Protected:  true,
			Capabilities: []string{
				"files",
				"db",
			},
		},
	})

	if len(entries) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(entries))
	}
	if entries[0].Name != "prod" {
		t.Fatalf("expected sorted remotes with prod first, got %#v", entries)
	}
	if !entries[0].Protected {
		t.Fatalf("expected prod remote to be protected")
	}
	if !reflect.DeepEqual(entries[0].Capabilities, []string{"files", "db"}) {
		t.Fatalf("unexpected capabilities for prod: %#v", entries[0].Capabilities)
	}
}

func TestDesktopPkgBuildRemoteAdminURLForTest(t *testing.T) {
	withConfiguredURL := desktop.BuildRemoteAdminURLForTest(
		desktop.RemoteConfigSnapshot{URL: "https://admin.remote.example/"},
		"backend_xyz",
	)
	if withConfiguredURL != "https://admin.remote.example/backend_xyz" {
		t.Fatalf("unexpected URL with configured base: %s", withConfiguredURL)
	}

	withHostFallback := desktop.BuildRemoteAdminURLForTest(
		desktop.RemoteConfigSnapshot{Host: "staging.example.com"},
		"",
	)
	if withHostFallback != "https://staging.example.com/admin" {
		t.Fatalf("unexpected URL with host fallback: %s", withHostFallback)
	}
}

func TestDesktopPkgResolveRemoteNameForOpenForTest(t *testing.T) {
	remotes := map[string]desktop.RemoteConfigSnapshot{
		"development": {
			Host:         "dev.example.com",
			Capabilities: []string{"files", "db"},
		},
		"production": {
			Host:         "prod.example.com",
			Capabilities: []string{"files"},
		},
	}

	resolved, err := desktop.ResolveRemoteNameForOpenForTest(remotes, "dev")
	if err != nil {
		t.Fatalf("unexpected error resolving dev alias: %v", err)
	}
	if resolved != "development" {
		t.Fatalf("expected development remote, got %s", resolved)
	}

	_, err = desktop.ResolveRemoteNameForOpenForTest(
		map[string]desktop.RemoteConfigSnapshot{
			"staging": {
				Host:         "staging.example.com",
				Capabilities: []string{"db"},
			},
		},
		"staging",
	)
	if err == nil {
		t.Fatalf("expected error when files capability is missing")
	}
}

func TestDesktopPkgNormalizeRemoteSyncPresetForTest(t *testing.T) {
	cases := map[string]string{
		"file":     "files",
		"files":    "files",
		"media":    "media",
		"db":       "db",
		"database": "db",
		"full":     "full",
	}

	for input, expected := range cases {
		value, err := desktop.NormalizeRemoteSyncPresetForTest(input)
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", input, err)
		}
		if value != expected {
			t.Fatalf("expected preset %s to normalize to %s, got %s", input, expected, value)
		}
	}

	if _, err := desktop.NormalizeRemoteSyncPresetForTest("unknown"); err == nil {
		t.Fatal("expected invalid preset to return error")
	}
}

func TestDesktopPkgBuildRemoteSyncPlanArgsForTest(t *testing.T) {
	args, err := desktop.BuildRemoteSyncPlanArgsForTest("staging", "media")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"sync", "--source", "staging", "--destination", "local", "--media", "--plan"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected sync args: %#v", args)
	}
}

func TestDesktopPkgBuildRemoteSyncPlanArgsWithOptionsForTest(t *testing.T) {
	args, err := desktop.BuildRemoteSyncPlanArgsWithOptionsForTest(
		"staging",
		"files",
		true,
		true,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, expectedArg := range []string{
		"sync",
		"--source",
		"staging",
		"--destination",
		"local",
		"--file",
		"--exclude",
		".env",
		"var/log/**",
		"--no-compress",
		"--plan",
	} {
		if !containsString(args, expectedArg) {
			t.Fatalf("expected %q in sync args: %#v", expectedArg, args)
		}
	}
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func TestBuildPresetSyncOptionDefs_DB(t *testing.T) {
	opts := desktop.BuildPresetSyncOptionDefsForTest("db")
	if opts.Command != "sync" {
		t.Fatalf("expected command 'sync', got %s", opts.Command)
	}
	if len(opts.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(opts.Options))
	}
	if opts.Options[0].Key != "compress" || opts.Options[1].Key != "noStreamDb" {
		t.Fatalf("unexpected options for db preset")
	}
}

func TestBuildPresetSyncOptionDefs_Media(t *testing.T) {
	opts := desktop.BuildPresetSyncOptionDefsForTest("media")
	if opts.Command != "sync" {
		t.Fatalf("expected command 'sync', got %s", opts.Command)
	}
	if len(opts.Options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(opts.Options))
	}
	if opts.Options[0].Key != "compress" || opts.Options[1].Key != "includeProduct" || opts.Options[2].Key != "delete" {
		t.Fatalf("unexpected options for media preset")
	}
}

func TestBuildPresetSyncOptionDefs_Full(t *testing.T) {
	opts := desktop.BuildPresetSyncOptionDefsForTest("full")
	if opts.Command != "bootstrap" {
		t.Fatalf("expected command 'bootstrap', got %s", opts.Command)
	}
	if len(opts.Options) != 8 {
		t.Fatalf("expected 8 options, got %d", len(opts.Options))
	}
}

func TestBuildBootstrapArgsWithOptions(t *testing.T) {
	t.Run("Execution Mode", func(t *testing.T) {
		args, err := desktop.BuildBootstrapArgsWithOptionsForTest("staging", map[string]bool{
			"noDb":           true,
			"assumeYes":      true,
			"includeProduct": true,
		}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{"bootstrap", "--environment", "staging", "--no-db", "--include-product", "--yes"}
		if !reflect.DeepEqual(args, expected) {
			t.Fatalf("expected args %v, got %v", expected, args)
		}
	})

	t.Run("Plan Mode", func(t *testing.T) {
		args, err := desktop.BuildBootstrapArgsWithOptionsForTest("staging", map[string]bool{}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !containsString(args, "--plan") {
			t.Fatalf("expected --plan in args: %v", args)
		}
	})
}

func TestDesktopPkgBuildRemoteEntriesWithLastSyncForTest(t *testing.T) {
	entries := desktop.BuildRemoteEntriesWithLastSyncForTest(
		map[string]desktop.RemoteConfigSnapshot{
			"development": {
				Host: "dev.example.com",
			},
		},
		map[string]string{
			"dev": "2m ago",
		},
	)

	if len(entries) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(entries))
	}
	if entries[0].Environment != "dev" {
		t.Fatalf("expected normalized environment dev, got %s", entries[0].Environment)
	}
	if entries[0].LastSync != "2m ago" {
		t.Fatalf("expected last sync '2m ago', got %q", entries[0].LastSync)
	}
}

func TestDesktopPkgBuildRemoteLastSyncLabelsFromEventsForTest(t *testing.T) {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	project := "magento2-test-instance"
	events := []engine.OperationEvent{
		{
			Timestamp: now.Add(-4 * time.Minute).Format(time.RFC3339Nano),
			Operation: "sync.run",
			Status:    engine.OperationStatusSuccess,
			Project:   project,
			Source:    "development",
		},
		{
			Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
			Operation: "sync.run",
			Status:    engine.OperationStatusSuccess,
			Project:   project,
			Source:    "development",
		},
		{
			Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
			Operation: "bootstrap.run",
			Status:    engine.OperationStatusSuccess,
			Project:   project,
			Source:    "production",
		},
		{
			Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			Operation: "sync.run",
			Status:    engine.OperationStatusFailure,
			Project:   project,
			Source:    "staging",
		},
		{
			Timestamp: now.Add(-20 * time.Second).Format(time.RFC3339Nano),
			Operation: "proxy.start",
			Status:    engine.OperationStatusSuccess,
			Project:   project,
			Source:    "development",
		},
		{
			Timestamp: now.Add(-30 * time.Second).Format(time.RFC3339Nano),
			Operation: "sync.run",
			Status:    engine.OperationStatusSuccess,
			Project:   "other-project",
			Source:    "development",
		},
		{
			Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			Operation: "sync.run",
			Status:    engine.OperationStatusSuccess,
			Project:   project,
			Source:    "local",
		},
	}

	labels := desktop.BuildRemoteLastSyncLabelsFromEventsForTest(project, events, now)
	expected := map[string]string{
		"dev":  "2m ago",
		"prod": "2h ago",
	}
	if !reflect.DeepEqual(labels, expected) {
		t.Fatalf("unexpected labels: got %#v, want %#v", labels, expected)
	}
}
