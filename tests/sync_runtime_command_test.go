package tests

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
)

// TestSyncSandboxSourceResolvesWithoutAConfiguredBlock is the sync half of
// #470. `sync` is the one command in this file that reaches ResolveAutoRemote
// on every non-local source, so a project with a running sandbox and no
// `remotes.sandbox` block could not sync from its own rehearsal target at all:
// the resolver answered "remote 'sandbox' is not configured" before any
// endpoint was built.
//
// The `staging` remote in the fixture is what makes the assertion meaningful:
// a resolver that silently substituted a configured remote would pass a weaker
// test, and this one names the container's own target and nothing else.
func TestSyncSandboxSourceResolvesWithoutAConfiguredBlock(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	restore := cmd.StubSandboxResolverForTest(func(_ context.Context, _ string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return overlayBase(), deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "sandbox",
		"--destination", "local",
		"--file",
		"--path", "app/code",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync from the synthetic sandbox failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, deploy.SandboxUser+"@127.0.0.1:") {
		t.Fatalf("rsync did not target the sandbox container; log shows:\n%s", logs)
	}
	if !strings.Contains(logs, "/home/deployer/public_html/app/code") {
		t.Fatalf("rsync did not use the container's document root; log shows:\n%s", logs)
	}
	if strings.Contains(logs, "staging.example.com") {
		t.Fatalf("the configured staging remote was used instead of the sandbox; log shows:\n%s", logs)
	}
}

// TestSyncSandboxPreflightProbesTheContainerIdentity is Review Focus 1 on the
// sync path: a `remotes.sandbox` block that describes another machine must not
// decide what the transfer connects to. The transfer is built from the endpoint
// the resolver produced, so the hostile host, user and path have to be absent
// from the whole invocation — not just from the resolved config.
func TestSyncSandboxPreflightProbesTheContainerIdentity(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	restore := cmd.StubSandboxResolverForTest(func(_ context.Context, _ string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return overlayBase(), deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  sandbox:
    host: legacy.example.com
    user: intruder
    port: 2222
    path: /wrong/place
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "sandbox",
		"--destination", "local",
		"--file",
		"--path", "app/code",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync from the configured sandbox failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, deploy.SandboxUser+"@127.0.0.1:") {
		t.Fatalf("the pre-flight did not probe the container identity; log shows:\n%s", logs)
	}
	if strings.Contains(logs, "legacy.example.com") || strings.Contains(logs, "intruder@") {
		t.Fatalf("the transfer was built from the configured block, not the container; log shows:\n%s", logs)
	}
	if !strings.Contains(logs, "/home/deployer/public_html/app/code") {
		t.Fatalf("the transfer did not use the container's document root; log shows:\n%s", logs)
	}
}

// TestSyncSandboxDestinationResolvesWithoutAConfiguredBlock pins the other
// end of the same claim the reference pages make. `--destination` never went
// through ResolveAutoRemote, so it has always resolved `sandbox` through
// ensureRemoteKnown — which means the source and the destination were answering
// the same question two different ways, and only one of them was fixed by
// changing the resolver. Asserting the destination keeps the two halves honest:
// if a future change routes it through the config-only path again, this fails.
func TestSyncSandboxDestinationResolvesWithoutAConfiguredBlock(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	restore := cmd.StubSandboxResolverForTest(func(_ context.Context, _ string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return overlayBase(), deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "local",
		"--destination", "sandbox",
		"--file",
		"--path", "app/code",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync to the synthetic sandbox failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, deploy.SandboxUser+"@127.0.0.1:") {
		t.Fatalf("rsync did not target the sandbox container; log shows:\n%s", logs)
	}
}

// TestSyncSandboxAbsentPropagatesTheResolverError is the exit code the reference
// pages promise, measured through the command rather than the function: a
// Docker-capable host with no container is a plain execution failure (exit 1)
// whose message names `govard sandbox up`, not a "not configured" complaint and
// not a capability error. The Docker-less case is the sibling assertion in
// remote_test.go, where the probe itself lives.
func TestSyncSandboxAbsentPropagatesTheResolverError(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	restore := cmd.StubSandboxResolverForTest(func(_ context.Context, _ string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf(
			"unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it")
	})
	defer restore()

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "sandbox",
		"--destination", "local",
		"--file",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("sync error = <nil>, want the missing sandbox reported")
	}
	if !strings.Contains(err.Error(), "no sandbox exists") {
		t.Errorf("err = %v, want the resolver's own message propagated through the command", err)
	}
	if code := cli.Code(err); code != cli.CodeError {
		t.Errorf("cli.Code(err) = %d, want %d: a host with Docker and no container is an execution failure, not a missing capability", code, cli.CodeError)
	}
	if logs := readRuntimeLog(t, logPath); logs != "" {
		t.Errorf("no transfer may run without a container; rsync log shows:\n%s", logs)
	}
}

func TestSyncCommandRuntimeFileOnlyUsesRsyncShim(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "staging",
		"--destination", "local",
		"--file",
		"--path", "app/code",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync runtime failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, "rsync|-avz") {
		t.Fatalf("missing rsync invocation in log:\n%s", logs)
	}
	if !strings.Contains(logs, "--partial --append-verify") {
		t.Fatalf("expected resumable flags in rsync invocation:\n%s", logs)
	}
	if !strings.Contains(logs, "app/code") {
		t.Fatalf("expected path filter in rsync invocation:\n%s", logs)
	}
}

func TestSyncCommandRuntimeNoResumeNoCompressFlags(t *testing.T) {
	resetSyncFlagsForRuntimeTest(t)

	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "rsync.log")
	installSyncRuntimeRsyncShim(t, shimDir)
	t.Setenv("SYNC_RUNTIME_RSYNC_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sync",
		"--yes",
		"--source", "staging",
		"--destination", "local",
		"--file",
		"--no-resume",
		"--no-compress",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync runtime with no-resume/no-compress failed: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	if !strings.Contains(logs, "rsync|-av ") {
		t.Fatalf("expected non-compressed rsync mode (-av), got:\n%s", logs)
	}
	if strings.Contains(logs, "--append-verify") || strings.Contains(logs, "--partial") {
		t.Fatalf("did not expect resume flags with --no-resume, got:\n%s", logs)
	}
}

func installSyncRuntimeRsyncShim(t *testing.T, shimDir string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
log="${SYNC_RUNTIME_RSYNC_LOG:-}"
if [ -n "$log" ]; then
  printf 'rsync|%s\n' "$*" >> "$log"
fi
exit 0
`
	path := filepath.Join(shimDir, "rsync")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write rsync shim: %v", err)
	}
}

func resetSyncFlagsForRuntimeTest(t *testing.T) {
	t.Helper()
	cmd.ResetSyncFlagsForTest()
	t.Cleanup(cmd.ResetSyncFlagsForTest)
}
