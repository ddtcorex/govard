package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopPkgBuildRemoteEntriesForTest(t *testing.T) {
	entries := desktop.BuildRemoteEntriesForTest(map[string]desktop.RemoteConfigSnapshot{
		"staging": {
			Host:        "staging.example.com",
			User:        "deploy",
			Path:        "/var/www/staging",
			Port:        22,
			Environment: "staging",
			AuthMethod:  "keychain",
			Capabilities: []string{
				"files",
				"media",
				"db",
			},
		},
		"prod": {
			Host:        "prod.example.com",
			User:        "root",
			Path:        "/srv/www/prod",
			Port:        2222,
			Environment: "prod",
			AuthMethod:  "ssh-agent",
			Protected:   true,
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

func TestDesktopPkgListAndUpsertProjectRemotesForPathForTest(t *testing.T) {
	root := t.TempDir()
	baseConfig := strings.TrimSpace(`
project_name: demo

domain: demo.test

remotes:
  staging:
    host: stage.example.com
    user: deploy
    path: /var/www/stage
    environment: staging
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(baseConfig), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	before, err := desktop.ListProjectRemotesForPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes before upsert: %v", err)
	}
	if len(before.Remotes) != 1 {
		t.Fatalf("expected one remote before upsert, got %d", len(before.Remotes))
	}

	err = desktop.UpsertProjectRemoteForPathForTest(root, desktop.RemoteUpsertInput{
		Name:         "prod",
		Host:         "prod.example.com",
		User:         "root",
		Path:         "/srv/www/prod",
		Port:         2200,
		Environment:  "production",
		Capabilities: "files,db",
		AuthMethod:   "ssh_agent",
		Protected:    true,
	})
	if err != nil {
		t.Fatalf("upsert remote: %v", err)
	}

	after, err := desktop.ListProjectRemotesForPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes after upsert: %v", err)
	}
	if len(after.Remotes) != 2 {
		t.Fatalf("expected two remotes after upsert, got %d", len(after.Remotes))
	}

	foundProd := false
	for _, remote := range after.Remotes {
		if remote.Name != "prod" {
			continue
		}
		foundProd = true
		if remote.Environment != "prod" {
			t.Fatalf("expected normalized environment prod, got %s", remote.Environment)
		}
		if remote.AuthMethod != "ssh-agent" {
			t.Fatalf("expected normalized auth method ssh-agent, got %s", remote.AuthMethod)
		}
		if !reflect.DeepEqual(remote.Capabilities, []string{"files", "db"}) {
			t.Fatalf("unexpected prod capabilities: %#v", remote.Capabilities)
		}
	}
	if !foundProd {
		t.Fatal("expected prod remote after upsert")
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
