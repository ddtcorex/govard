package tests

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pterm/pterm"
	"govard/internal/cmd"
	"govard/internal/engine/tunnel"
	"govard/internal/runtime"
)

// narrowingProjectYAML is a neutral project with one remote, enough for sync
// and the remote commands to resolve their targets.
const narrowingProjectYAML = `project_name: sample-project
domain: sample-project.test
framework: laravel
remotes:
  dev:
    host: dev.example.invalid
    user: deploy
    path: /srv/app
`

// capabilityHostForTest isolates one command-level test from the developer's
// machine: a temp project, temp state directories, and failing shims for the
// three host tools these commands shell out to, so nothing reaches a real
// container, SSH server or rsync. missing names the executables the capability
// probe must not find.
func capabilityHostForTest(t *testing.T, missing ...string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(narrowingProjectYAML), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
	chdirForTest(t, dir)
	state := t.TempDir()
	t.Setenv("HOME", state)
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(state, "govard-home"))
	t.Setenv("GOVARD_OPERATIONS_LOG_PATH", filepath.Join(state, "operations.log"))
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", filepath.Join(state, "remote.log"))

	shimDir := t.TempDir()
	for _, name := range []string{"docker", "ssh", "rsync"} {
		if err := os.WriteFile(filepath.Join(shimDir, name), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatalf("write %s shim: %v", name, err)
		}
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	restore := runtime.StubLookPathForTest(func(name string) (string, error) {
		for _, gone := range missing {
			if name == gone {
				return "", exec.ErrNotFound
			}
		}
		return exec.LookPath(name)
	})
	t.Cleanup(restore)
}

func runCapabilityCommand(t *testing.T, args ...string) error {
	t.Helper()
	pterm.SetDefaultOutput(io.Discard)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	root := cmd.RootCommandForTest()
	t.Cleanup(snapshotCommandTreeFlags(t, root))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func requireMissingCapability(t *testing.T, err error, want runtime.Capability, what string) {
	t.Helper()
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("%s must answer CAPABILITY_MISSING for %s, got %v", what, want, err)
	}
	if missing.MissingCapability() != string(want) {
		t.Fatalf("%s names capability %q, want %q", what, missing.MissingCapability(), want)
	}
	if missing.ExitCode() != runtime.CodeCapabilityMissing {
		t.Fatalf("%s exit code = %d, want %d", what, missing.ExitCode(), runtime.CodeCapabilityMissing)
	}
}

// #503 (comment): `sync` declares ssh,rsync, but --db and --full move the
// database through the local container. On a host without a container runtime
// they must answer exit 3 CAPABILITY_MISSING naming docker, not fail later with
// a raw "database container is not running".
func TestSyncDBRequiresContainerCapability(t *testing.T) {
	for _, args := range [][]string{
		{"sync", "-s", "dev", "--db", "--yes"},
		{"sync", "-s", "dev", "--full", "--yes"},
	} {
		t.Run(strings.Join(args[3:], " "), func(t *testing.T) {
			restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapSSH, runtime.CapRsync)
			t.Cleanup(restore)
			capabilityHostForTest(t, "docker")

			requireMissingCapability(t, runCapabilityCommand(t, args...), runtime.CapDocker, strings.Join(args, " "))
		})
	}

	// A plan only prints what would run and never touches the container, so it
	// stays available on the same host.
	t.Run("--db --plan", func(t *testing.T) {
		restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapSSH, runtime.CapRsync)
		t.Cleanup(restore)
		capabilityHostForTest(t, "docker")

		err := runCapabilityCommand(t, "sync", "-s", "dev", "--db", "--plan")
		var missing *runtime.MissingError
		if errors.As(err, &missing) {
			t.Fatalf("sync --db --plan must not be gated on a container runtime: %v", err)
		}
	})
}

// #503 (comment): `remote list` reads the project config and `remote audit
// stats|tail` read a local log, so none of them needs ssh or rsync.
func TestRemoteAuditTailNeedsNoRsync(t *testing.T) {
	for _, args := range [][]string{
		{"remote", "audit", "tail"},
		{"remote", "audit", "stats"},
		{"remote", "list"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			capabilityHostForTest(t, "ssh", "rsync")
			if err := runCapabilityCommand(t, args...); err != nil {
				t.Fatalf("%s must run without ssh and rsync: %v", strings.Join(args, " "), err)
			}
		})
	}

	// The rest of the group still talks to the remote: the same host is refused.
	t.Run("remote test stays gated", func(t *testing.T) {
		capabilityHostForTest(t, "ssh", "rsync")
		requireMissingCapability(t, runCapabilityCommand(t, "remote", "test", "dev"), runtime.CapSSH, "remote test")
	})
}

// #503: since #469, stop signals one recorded pid and status reads one record;
// neither runs cloudflared. They must answer on a host without it, while start,
// which does run it, keeps the gate.
func TestTunnelStopAndStatusNeedNoCloudflared(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	tunnelProjectForTest(t)
	restoreLookPath := runtime.StubLookPathForTest(func(name string) (string, error) {
		if name == "cloudflared" {
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	})
	t.Cleanup(restoreLookPath)

	for _, args := range [][]string{{"tunnel", "stop"}, {"tunnel", "status"}} {
		out, err := runTunnel(t, args...)
		if err != nil {
			t.Fatalf("%s must run without cloudflared: %v (out %q)", strings.Join(args, " "), err, out)
		}
	}

	// A fake provider that fails at once: if the start gate regressed, the start
	// must fail fast here instead of launching a real cloudflared on a host
	// that happens to have one.
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: fakePlanProvider(tunnel.StartPlan{Binary: "/bin/false"}),
	})
	defer restore()
	_, err := runTunnel(t, "tunnel", "start", "https://demo.trycloudflare.com")
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("tunnel start must stay gated on cloudflared, got %v", err)
	}
	if missing.MissingCapability() != string(runtime.CapCloudflared) {
		t.Fatalf("tunnel start names capability %q, want cloudflared", missing.MissingCapability())
	}
}
