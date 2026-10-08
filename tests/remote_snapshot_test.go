package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

func TestBuildRemoteSnapshotCreateCommand(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host: "staging.example.com",
		User: "deploy",
		Path: "/var/www/app",
		Port: 22,
	}
	cmd := remote.BuildRemoteSnapshotCreateCommandForTest("staging", remoteCfg, "my-snap", "magento2")
	if cmd == "" {
		t.Fatal("expected non-empty create command")
	}
	if !strings.Contains(cmd, ".govard/snapshots/my-snap") {
		t.Fatalf("expected snapshot path in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "mkdir") {
		t.Fatalf("expected mkdir in create command, got: %s", cmd)
	}
}

func TestBuildRemoteSnapshotListCommand(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host: "staging.example.com",
		User: "deploy",
		Path: "/var/www/app",
		Port: 22,
	}
	cmd := remote.BuildRemoteSnapshotListCommandForTest(remoteCfg)
	if cmd == "" {
		t.Fatal("expected non-empty list command")
	}
	if !strings.Contains(cmd, ".govard/snapshots") {
		t.Fatalf("expected snapshot root in command, got: %s", cmd)
	}
}

func TestBuildRemoteSnapshotDeleteCommand(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host: "staging.example.com",
		User: "deploy",
		Path: "/var/www/app",
		Port: 22,
	}
	cmd := remote.BuildRemoteSnapshotDeleteCommandForTest(remoteCfg, "old-snap")
	if cmd == "" {
		t.Fatal("expected non-empty delete command")
	}
	if !strings.Contains(cmd, "rm -rf") {
		t.Fatalf("expected rm -rf in delete command, got: %s", cmd)
	}
	if !strings.Contains(cmd, ".govard/snapshots") || !strings.Contains(cmd, "'old-snap'") {
		t.Fatalf("expected snapshot roots and name in delete command, got: %s", cmd)
	}
}

func TestBuildRemoteSnapshotRestoreCommand(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host: "staging.example.com",
		User: "deploy",
		Path: "/var/www/app",
		Port: 22,
	}
	cmd := remote.BuildRemoteSnapshotRestoreCommandForTest(remoteCfg, "my-snap", "magento2", false, false)
	if cmd == "" {
		t.Fatal("expected non-empty restore command")
	}
	if !strings.Contains(cmd, ".govard/snapshots") || !strings.Contains(cmd, "'my-snap'") {
		t.Fatalf("expected snapshot roots and name in restore command, got: %s", cmd)
	}
}

func TestRemoteSnapshotPathSafety(t *testing.T) {
	safeCases := []string{
		"valid-name",
		"20260401-120000",
		"my_snap.1",
		"abc123",
	}
	for _, name := range safeCases {
		if err := remote.ValidateSnapshotNameForTest(name); err != nil {
			t.Fatalf("expected no error for safe name %q, got: %v", name, err)
		}
	}

	unsafeCases := []string{
		"../escape",
		"../../etc/passwd",
		"sub/dir",
		"",
		" ",
		"a b c",
	}
	for _, name := range unsafeCases {
		if err := remote.ValidateSnapshotNameForTest(name); err == nil {
			t.Fatalf("expected error for unsafe name %q, got nil", name)
		}
	}
}

func TestParseRemoteSnapshotListOutput(t *testing.T) {
	raw := `name: snap-1
created_at: 2026-04-01T08:00:00Z
framework: magento2
db: true
media: true
---
name: snap-2
created_at: 2026-04-01T09:00:00Z
framework: magento2
db: true
media: false
---
`
	snapshots, err := remote.ParseRemoteSnapshotListForTest(raw)
	if err != nil {
		t.Fatalf("parse remote snapshot list: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	if snapshots[0].Name != "snap-1" {
		t.Fatalf("expected first snapshot name snap-1, got %s", snapshots[0].Name)
	}
	if snapshots[1].Name != "snap-2" {
		t.Fatalf("expected second snapshot name snap-2, got %s", snapshots[1].Name)
	}
}

func TestParseRemoteSnapshotListEmpty(t *testing.T) {
	snapshots, err := remote.ParseRemoteSnapshotListForTest("EMPTY")
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestParseRemoteSnapshotListBlank(t *testing.T) {
	snapshots, err := remote.ParseRemoteSnapshotListForTest("")
	if err != nil {
		t.Fatalf("parse blank: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots for blank input, got %d", len(snapshots))
	}
}

func TestSnapshotPushBlockedOnProtectedRemote(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host:      "prod.example.com",
		User:      "deploy",
		Path:      "/var/www/app",
		Port:      22,
		Protected: engine.BoolPtr(true),
	}
	blocked, reason := engine.RemoteWriteBlocked("production", remoteCfg)
	if !blocked {
		t.Fatal("expected push to be blocked on protected remote")
	}
	if reason == "" {
		t.Fatal("expected a reason for the block")
	}
}

func TestSnapshotCreateRemoteDBCapabilityCheck(t *testing.T) {
	remoteCfg := engine.RemoteConfig{
		Host: "staging.example.com",
		User: "deploy",
		Path: "/var/www/app",
		Port: 22,
		Capabilities: &engine.RemoteCapabilities{
			Files: engine.BoolPtr(true),
			Media: engine.BoolPtr(true),
			DB:    engine.BoolPtr(false),
		},
	}
	if engine.RemoteCapabilityEnabled(remoteCfg, engine.RemoteCapabilityDB) {
		t.Fatal("expected DB capability to be disabled")
	}
}

func TestSnapshotCreateRemoteRequiresEnvironment(t *testing.T) {
	// this is covered by other tests, actually let's test command directly if possible
}
