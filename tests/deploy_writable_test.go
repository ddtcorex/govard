package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func writableContext(t *testing.T, settings map[string]any) (*deploy.StepContext, string) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")
	target := filepath.Join(release.Path, "var")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("prepare writable dir: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:         "local",
		CommandTimeout: 0,
		Settings:       settings,
	})
	sc.Release = release
	sc.Runner = captureRunner{base: deploy.LocalRunner{}, seen: new([]string)}
	return sc, target
}

func TestWritableAppliesChmodByDefault(t *testing.T) {
	sc, target := writableContext(t, map[string]any{"writable_dirs": []string{"var"}})

	if err := deploy.CoreWritable(context.Background(), sc); err != nil {
		t.Fatalf("writable: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o775 {
		t.Fatalf("mode = %v, want 0775", info.Mode().Perm())
	}
}

func TestWritableSkipDoesNothing(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "skip",
	})

	if err := deploy.CoreWritable(context.Background(), sc); err != nil {
		t.Fatalf("writable skip: %v", err)
	}
	// A skip that still ran commands would be a lie in the plan output.
	if commands := *sc.Runner.(captureRunner).seen; len(commands) != 0 {
		t.Fatalf("writable_mode=skip ran %v", commands)
	}
}

func TestWritableChownNeedsAConfiguredOwner(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "chmod+chown",
	})

	err := deploy.CoreWritable(context.Background(), sc)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("err = %v, want a refusal naming settings.owner", err)
	}
}

func TestWritableRejectsAnUnknownMode(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "teleport",
	})

	if err := deploy.CoreWritable(context.Background(), sc); err == nil {
		t.Fatal("want a refusal for an unknown writable_mode")
	}
}
