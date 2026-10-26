package tests

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

type ownershipFixRecorder struct {
	chownCalls [][]engine.OwnedPath
	mountReads int
	httpCalls  int
}

func ownershipFixDeps(t *testing.T, rec *ownershipFixRecorder) (cmd.GovardOwnershipFixDepsForTest, string) {
	t.Helper()
	root := t.TempDir()
	govardDir := filepath.Join(root, ".govard")
	config := engine.Config{ProjectName: "sample-project", Domain: "sample-project.test", Framework: "magento2"}
	config.Stack.Services.WebServer = "apache"
	return cmd.GovardOwnershipFixDepsForTest{
		ProjectRoot:      func() (string, error) { return root, nil },
		LoadConfig:       func(string) (engine.Config, error) { return config, nil },
		CurrentIDs:       func() (int, int) { return 1000, 1000 },
		RuntimeAvailable: func() bool { return true },
		Foreign: func(string, int) ([]engine.OwnedPath, error) {
			return []engine.OwnedPath{
				{Path: govardDir, UID: 0, GID: 0},
				{Path: filepath.Join(govardDir, "apache"), UID: 0, GID: 0},
				{Path: filepath.Join(govardDir, "link"), UID: 0, GID: 0},
			}, nil
		},
		Chown: func(_ context.Context, _ string, entries []engine.OwnedPath) error {
			rec.chownCalls = append(rec.chownCalls, append([]engine.OwnedPath(nil), entries...))
			return nil
		},
		ProbeWrite: func(string) error { return nil },
		EnvRunning: func(string, engine.Config) bool { return true },
		ReadMount: func(context.Context, string, string) error {
			rec.mountReads++
			return nil
		},
		HTTPStatus: func(context.Context, string) (int, error) {
			rec.httpCalls++
			return 200, nil
		},
	}, govardDir
}

func runOwnershipFix(t *testing.T, deps cmd.GovardOwnershipFixDepsForTest) cmd.DoctorFixResult {
	t.Helper()
	restore := cmd.SetGovardOwnershipFixDepsForTest(deps)
	defer restore()
	return cmd.FixGovardOwnershipForTest(engine.DoctorCheck{ID: "project.govard.ownership", Title: ".govard ownership"})
}

func TestGovardOwnershipFixUnavailableWithoutRuntime(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, _ := ownershipFixDeps(t, rec)
	deps.RuntimeAvailable = func() bool { return false }
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusUnavailable || len(rec.chownCalls) != 0 {
		t.Fatalf("result %#v chown calls %d, want unavailable without chown", result, len(rec.chownCalls))
	}
}

func TestGovardOwnershipFixAppliesAndListsPaths(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, govardDir := ownershipFixDeps(t, rec)
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusApplied {
		t.Fatalf("status = %q (%s), want applied", result.Status, result.Message)
	}
	if len(rec.chownCalls) != 1 {
		t.Fatalf("chown calls = %d, want 1", len(rec.chownCalls))
	}
	for _, entry := range rec.chownCalls[0] {
		if entry.UID != 1000 || entry.GID != 1000 {
			t.Fatalf("entry %#v not given to uid:gid 1000:1000", entry)
		}
	}
	joined := strings.Join(result.Actions, "\n")
	for _, rel := range []string{".govard", filepath.Join(".govard", "apache")} {
		if !strings.Contains(joined, rel) {
			t.Fatalf("actions %q do not list %s", joined, rel)
		}
	}
	if rec.mountReads != 1 || rec.httpCalls != 1 {
		t.Fatalf("mount reads %d http calls %d, want the live post-checks to run", rec.mountReads, rec.httpCalls)
	}
	_ = govardDir
}

func TestGovardOwnershipFixRollsBackOnProbeFailure(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, govardDir := ownershipFixDeps(t, rec)
	deps.ProbeWrite = func(string) error { return errors.New("permission denied") }
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusFailed || !strings.Contains(result.Message, "write probe") {
		t.Fatalf("result = %#v, want failed naming the write probe", result)
	}
	if len(rec.chownCalls) != 2 {
		t.Fatalf("chown calls = %d, want the fix and the rollback", len(rec.chownCalls))
	}
	for _, entry := range rec.chownCalls[1] {
		if entry.UID != 0 || entry.GID != 0 {
			t.Fatalf("rollback entry %#v, want the original root ownership", entry)
		}
	}
	_ = govardDir
}

func TestGovardOwnershipFixRollsBackOnHTTPFailure(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, _ := ownershipFixDeps(t, rec)
	deps.HTTPStatus = func(context.Context, string) (int, error) { return 502, nil }
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusFailed || !strings.Contains(result.Message, "502") || len(rec.chownCalls) != 2 {
		t.Fatalf("result = %#v calls %d, want failed with 502 and a rollback", result, len(rec.chownCalls))
	}
}

func TestGovardOwnershipFixSkipsLiveChecksWhenEnvDown(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, _ := ownershipFixDeps(t, rec)
	deps.EnvRunning = func(string, engine.Config) bool { return false }
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusApplied || rec.mountReads != 0 || rec.httpCalls != 0 {
		t.Fatalf("result %#v mount %d http %d, want applied without live checks", result, rec.mountReads, rec.httpCalls)
	}
	if !strings.Contains(strings.Join(result.Actions, "\n"), "environment down: mount and HTTP checks skipped") {
		t.Fatalf("actions %v must say the live checks were skipped", result.Actions)
	}
}

func TestGovardOwnershipFixDaemonDownLeavesOwnership(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, _ := ownershipFixDeps(t, rec)
	deps.Chown = func(context.Context, string, []engine.OwnedPath) error {
		rec.chownCalls = append(rec.chownCalls, nil)
		return errors.New("Cannot connect to the Docker daemon")
	}
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusFailed || len(rec.chownCalls) != 1 || !strings.Contains(result.Message, "Docker daemon") {
		t.Fatalf("result %#v calls %d, want failed once with the daemon error", result, len(rec.chownCalls))
	}
}

func TestGovardOwnershipFixNeverTouchesOutsideGovard(t *testing.T) {
	rec := &ownershipFixRecorder{}
	deps, govardDir := ownershipFixDeps(t, rec)
	deps.Foreign = func(string, int) ([]engine.OwnedPath, error) {
		return []engine.OwnedPath{{Path: govardDir, UID: 0}, {Path: "/etc/passwd", UID: 0}}, nil
	}
	result := runOwnershipFix(t, deps)
	if result.Status != cmd.DoctorFixStatusFailed || len(rec.chownCalls) != 0 {
		t.Fatalf("result %#v calls %d, want refusal before any chown", result, len(rec.chownCalls))
	}
}

func TestGovardOwnershipChownScriptUsesNoDereference(t *testing.T) {
	script, err := cmd.GovardOwnershipChownScriptForTest("/p/.govard", []engine.OwnedPath{
		{Path: "/p/.govard", UID: 1000, GID: 1000},
		{Path: "/p/.govard/it's", UID: 1000, GID: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "chown -h 1000:1000 '/d'") || !strings.Contains(script, `chown -h 1000:1000 '/d/it'\''s'`) {
		t.Fatalf("script = %q, want quoted chown -h per path", script)
	}
	if strings.Contains(script, "-R") || strings.Contains(script, "-L") {
		t.Fatalf("script = %q must never recurse or follow links", script)
	}
}
