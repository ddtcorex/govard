package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// TestResolveAutoRemoteSandboxAbsent is the completion of the 2026-09-17
// migration. Its task list hooked four call sites and never ResolveAutoRemote,
// so every consumer on the config-only path answered "remote 'sandbox' is not
// configured" for the one remote name that is resolvable without configuration.
// The remedy is in the message, so the message is the behaviour under test.
func TestResolveAutoRemoteSandboxAbsent(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf(
			"unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it")
	})
	defer restore()

	name, err := cmd.ResolveAutoRemote(engine.Config{ProjectName: "sample-project"}, "sandbox")
	if err == nil {
		t.Fatalf("ResolveAutoRemote() = %q, <nil>, want the sandbox resolver's own message", name)
	}
	if !strings.Contains(err.Error(), "no sandbox exists") || !strings.Contains(err.Error(), "run 'govard sandbox up'") {
		t.Errorf("err = %v, want the resolver's own message propagated unchanged", err)
	}
	if strings.Contains(err.Error(), "is not configured") {
		t.Errorf("err = %v, want the sandbox's message, not the config-only one", err)
	}
}

// TestResolveAutoRemoteSandboxOnADockerlessHost is Review Focus 4, and the
// reason this function is no longer pure: `sync` declares itself Docker-free
// (ssh,rsync), so a Docker-less host reaches this resolver and has to be told
// the truth about what is missing. Exit 3 with the capability envelope, not
// exit 1 with a message about a configuration file that is not the problem.
//
// The real resolver is used on purpose — the capability probe lives inside it,
// so a stubbed resolver would assert only that the stub was called. Both docker
// probes are stubbed: probeOne currently stops after the status check, but that
// ordering is a property of the probe rather than of this test.
func TestResolveAutoRemoteSandboxOnADockerlessHost(t *testing.T) {
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return errors.New("Cannot connect to the Docker daemon") },
		func(context.Context) error { return errors.New("Cannot connect to the Docker daemon") },
	)
	defer restore()

	// The block is deliberately present: a config hit must not short-circuit
	// the probe, which is the whole ordering decision of this change.
	config := engine.Config{
		ProjectName: "sample-project",
		Remotes:     engine.RemoteConfigMap{"sandbox": {Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)}}},
	}

	_, err := cmd.ResolveAutoRemote(config, "sandbox")
	if err == nil {
		t.Fatal("ResolveAutoRemote() error = <nil>, want the Docker probe to fail a Docker-less host")
	}
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v (%T), want a *runtime.MissingError, not the config-only message", err, err)
	}
	if len(missing.Caps) != 1 || missing.Caps[0] != runtime.CapDocker {
		t.Errorf("missing.Caps = %v, want [%s]", missing.Caps, runtime.CapDocker)
	}
	if code := cli.Code(err); code != cli.CodeCapability {
		t.Errorf("cli.Code(err) = %d, want %d (CAPABILITY_MISSING)", code, cli.CodeCapability)
	}
	if strings.Contains(err.Error(), "is not configured") {
		t.Errorf("err = %v, want the capability message, not the config-only one", err)
	}
}

// TestResolveAutoRemoteSandboxConfiguredStillRequiresLiveState is the
// consequence Task 1's block made visible: a configured `remotes.sandbox` now
// reaches the resolver, and a config hit must not end resolution. The block
// layers shape over identity, it does not replace the container that supplies
// the identity — so a dormant container has to surface its own message rather
// than resolve to a name whose remote does not exist.
func TestResolveAutoRemoteSandboxConfiguredStillRequiresLiveState(t *testing.T) {
	config := engine.Config{
		ProjectName: "sample-project",
		Remotes:     engine.RemoteConfigMap{"sandbox": {Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)}}},
	}

	t.Run("running", func(t *testing.T) {
		restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
			return overlayBase(), deploy.SandboxLivenessRunning, nil
		})
		defer restore()

		name, err := cmd.ResolveAutoRemote(config, "sandbox")
		if err != nil {
			t.Fatalf("ResolveAutoRemote() error = %v, want the configured sandbox to resolve", err)
		}
		if name != "sandbox" {
			t.Errorf("ResolveAutoRemote() = %q, want %q", name, "sandbox")
		}
	})

	t.Run("dormant", func(t *testing.T) {
		restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
			return engine.RemoteConfig{}, deploy.SandboxLivenessDormant, fmt.Errorf(
				"sandbox container govard-sample-sandbox-abc123 is not running; run 'govard sandbox up' to start it")
		})
		defer restore()

		name, err := cmd.ResolveAutoRemote(config, "sandbox")
		if err == nil {
			t.Fatalf("ResolveAutoRemote() = %q, <nil>, want the dormant container's message", name)
		}
		if !strings.Contains(err.Error(), "is not running") {
			t.Errorf("err = %v, want the dormant message propagated unchanged", err)
		}
		if strings.Contains(err.Error(), "is not configured") {
			t.Errorf("err = %v, want the dormant message, not the config-only one", err)
		}
	})
}

// TestResolveAutoRemoteSandboxIsMatchedCaseInsensitively closes the last way a
// config hit could end resolution for the synthetic name. `findRemote…` scans
// the configured names for an environment alias, so a hand-edited "Sandbox:"
// answers a `sandbox` request — and every other sandbox name check in the
// codebase treats that spelling as the same remote. An exact-case comparison
// here would return "Sandbox" without ever asking the container, so a project
// carrying that spelling would report a resolution success that the next call
// then contradicts, with the accurate message arriving from somewhere else.
func TestResolveAutoRemoteSandboxIsMatchedCaseInsensitively(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(_ context.Context, _ string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessDormant, fmt.Errorf(
			"sandbox container govard-sample-sandbox-abc123 is not running; run 'govard sandbox up' to start it")
	})
	defer restore()

	config := engine.Config{
		ProjectName: "sample-project",
		Remotes:     engine.RemoteConfigMap{"Sandbox": {Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)}}},
	}

	name, err := cmd.ResolveAutoRemote(config, "sandbox")
	if err == nil {
		t.Fatalf("ResolveAutoRemote() = %q, <nil>, want the dormant container's message under any spelling of the block", name)
	}
	if !strings.Contains(err.Error(), "is not running") {
		t.Errorf("err = %v, want the dormant message: a \"Sandbox:\" block is the same block", err)
	}
}

func TestEnsureRemoteKnownResolvesSandboxWhenRunning(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{Host: "127.0.0.1", User: deploy.SandboxUser, Sandbox: true}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	name, remote, err := cmd.EnsureRemoteKnownForTest(engine.Config{ProjectName: "sample-project"}, "sandbox")
	if err != nil {
		t.Fatalf("ensureRemoteKnown: %v", err)
	}
	if name != "sandbox" || remote.Host != "127.0.0.1" {
		t.Fatalf("name=%q remote=%+v, want the synthetic sandbox remote", name, remote)
	}
}

func TestEnsureRemoteKnownSurfacesTheDormantError(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessDormant, fmt.Errorf("sandbox container govard-sample-sandbox-abc123 is not running; run 'govard sandbox up' to start it")
	})
	defer restore()

	_, _, err := cmd.EnsureRemoteKnownForTest(engine.Config{ProjectName: "sample-project"}, "sandbox")
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("err = %v, want the dormant message propagated unchanged", err)
	}
}

// TestEnsureRemoteKnownLayersConfiguredSandboxSettings proves the synthetic
// branch now resolves through the overlay: a configured block contributes its
// capabilities, while host, port, user and auth stay the container's — the
// resolution that opens an SSH session must never be a block's to change.
func TestEnsureRemoteKnownLayersConfiguredSandboxSettings(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return overlayBase(), deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	config := engine.Config{
		ProjectName: "sample-project",
		Remotes: engine.RemoteConfigMap{"sandbox": {
			Host:         "legacy.example.com",
			User:         "intruder",
			Port:         22,
			Path:         "/srv/legacy",
			Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)},
		}},
	}

	_, remote, err := cmd.EnsureRemoteKnownForTest(config, "sandbox")
	if err != nil {
		t.Fatalf("ensureRemoteKnown: %v", err)
	}
	if remote.Capabilities == nil || remote.Capabilities.DB == nil || *remote.Capabilities.DB {
		t.Fatalf("remote.Capabilities = %+v, want the configured db capability", remote.Capabilities)
	}
	if remote.Host != "127.0.0.1" || remote.User != deploy.SandboxUser || remote.Port != 32998 {
		t.Errorf("remote host/port/user = %q/%d/%q, want the container-derived 127.0.0.1/32998/%s", remote.Host, remote.Port, remote.User, deploy.SandboxUser)
	}
	if remote.Path != "/home/deployer/public_html" {
		t.Errorf("remote.Path = %q, want the container's document root", remote.Path)
	}
	if remote.Auth.KeyPath != ".govard/sandbox/id_ed25519" {
		t.Errorf("remote.Auth.KeyPath = %q, want the key `sandbox up` provisioned", remote.Auth.KeyPath)
	}
}

// TestResolvedRemoteForNameKeepsEachCallersSkipSemantics guards the contract
// the callers that migrated onto resolvedRemoteForName depend on: an unknown
// non-sandbox name is still "nothing configured" (skip, no error), while a
// synthetic sandbox that cannot be resolved is still a real error rather than
// an absence. The two failure shapes must not be collapsed into one — the
// first means "do not run this", the second means "run `govard sandbox up`".
func TestResolvedRemoteForNameKeepsEachCallersSkipSemantics(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf(
			"unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it")
	})
	defer restore()

	config := engine.Config{ProjectName: "sample-project"}

	remote, ok, err := cmd.ResolvedRemoteForNameForTest(context.Background(), config, "staging")
	if err != nil || ok {
		t.Errorf("unknown non-sandbox remote: ok = %v, err = %v, want (false, nil) so the caller skips", ok, err)
	}
	if remote.Host != "" {
		t.Errorf("unknown non-sandbox remote returned %+v, want the zero config", remote)
	}

	remote, ok, err = cmd.ResolvedRemoteForNameForTest(context.Background(), config, "sandbox")
	if err == nil || ok {
		t.Fatalf("missing sandbox: ok = %v, err = %v, want the resolver's own error, never a silent skip", ok, err)
	}
	if !strings.Contains(err.Error(), "run 'govard sandbox up'") {
		t.Errorf("err = %v, want the sandbox's own message propagated unchanged", err)
	}
}

// TestRemoteAddConfiguresTheSandbox inverts the refusal the 2026-09-17 plan
// added: a sandbox is the one remote whose *identity* cannot be configured, so
// `remote add sandbox` now writes the rehearsal's shape and drops every
// identity flag instead of rejecting the name. The fixture deliberately keeps
// the shape of the TestRemoteAddRefusesTheReservedSandboxName it replaces, so
// the reversal is legible in the diff.
func TestRemoteAddConfiguresTheSandbox(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: sample-project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "sandbox", "--capabilities", "db", "--host", "legacy.example.com", "--user", "intruder"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remote add sandbox: %v, want the block written: a sandbox is configurable, its identity is not", err)
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(written)
	if !strings.Contains(body, "capabilities:") {
		t.Fatalf(".govard.yml missing the configured capability block:\n%s", body)
	}
	// The block is parsed back rather than grepped for keys: RemoteConfig's
	// host/user/path/port carry no `omitempty`, so an ignored identity still
	// serialises as an empty key. The property under test is that no identity
	// *value* is stored, and only parsing can say that.
	var saved struct {
		Remotes map[string]engine.RemoteConfig `yaml:"remotes"`
	}
	if err := yaml.Unmarshal(written, &saved); err != nil {
		t.Fatalf("parse .govard.yml: %v\n%s", err, body)
	}
	block, ok := saved.Remotes["sandbox"]
	if !ok {
		t.Fatalf("remotes.sandbox was not written:\n%s", body)
	}
	if block.Host != "" || block.User != "" || block.Path != "" || block.Port != 0 {
		t.Errorf("stored identity host=%q user=%q path=%q port=%d, want all empty: the sandbox's identity is container-derived, never stored.\n%s",
			block.Host, block.User, block.Path, block.Port, body)
	}
	if block.Auth != (engine.RemoteAuth{}) {
		t.Errorf("stored auth = %+v, want empty: auth is not layerable either", block.Auth)
	}
	if block.Capabilities == nil || block.Capabilities.DB == nil || *block.Capabilities.DB {
		t.Errorf("stored capabilities = %+v, want db blocked by --capabilities db", block.Capabilities)
	}
}

// TestRemoteAddSandboxBlockSurvivesASaveWithoutGainingIdentity covers the
// second half of that promise, and the half the test above structurally
// cannot: `remote add` builds its entry *after* the config was loaded and
// normalised, so it wrote a correct block without ever running the port
// default. Every other save does the opposite order — load, normalise, then
// marshal the whole config back — so the default ran against the already-stored
// block and an unrelated command (`govard config set` here) rewrote it to assert
// `port: 22`, a port `sandbox up` never publishes. Nothing objected:
// validateOptionalRemoteFields explicitly permits a non-zero port in a sandbox
// block, and every reader routes through the overlay, so the false value was
// only ever visible in the file the operator reads.
//
// The assertion is on the bytes on disk after the round trip, not on the
// normaliser: the property is about what is persisted, and reading the
// normaliser would pass whether or not anything ever wrote it out.
func TestRemoteAddSandboxBlockSurvivesASaveWithoutGainingIdentity(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: sample-project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "sandbox", "--capabilities", "db", "--protected"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remote add sandbox: %v", err)
	}

	// An unrelated command that loads, normalises and saves the whole config.
	root.SetArgs([]string{"config", "set", "framework", "magento2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("config set: %v", err)
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(written)
	var saved struct {
		Remotes map[string]engine.RemoteConfig `yaml:"remotes"`
	}
	if err := yaml.Unmarshal(written, &saved); err != nil {
		t.Fatalf("parse .govard.yml: %v\n%s", err, body)
	}
	block, ok := saved.Remotes["sandbox"]
	if !ok {
		t.Fatalf("remotes.sandbox did not survive the save:\n%s", body)
	}
	if block.Host != "" || block.User != "" || block.Path != "" || block.Port != 0 {
		t.Errorf("stored identity after an unrelated save: host=%q user=%q path=%q port=%d, want all empty: a save must not hand the sandbox block an identity it never had.\n%s",
			block.Host, block.User, block.Path, block.Port, body)
	}
	if block.Capabilities == nil || block.Capabilities.DB == nil || *block.Capabilities.DB {
		t.Errorf("stored capabilities = %+v, want the block's own shape to survive the save", block.Capabilities)
	}
}

// TestPartialSandboxBlockLoads is the half-state the 2026-09-17 plan created: a
// block that says only what a rehearsal can legitimately state could not even
// be parsed, because the validator demanded the host of a remote that has no
// configured host to give.
func TestPartialSandboxBlockLoads(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  sandbox:
    capabilities:
      db: false
    protected: true
    deploy:
      keep_releases: 3
`)

	cfg, _, err := engine.LoadConfigFromDir(root, false)
	if err != nil {
		t.Fatalf("a shape-only remotes.sandbox block must load: %v", err)
	}
	block, ok := cfg.Remotes["sandbox"]
	if !ok {
		t.Fatal("remotes.sandbox did not survive validation")
	}
	if block.Host != "" || block.User != "" {
		t.Errorf("block host/user = %q/%q, want empty: the sandbox's identity stays container-derived", block.Host, block.User)
	}
}

// TestPartialNonSandboxRemoteStillRejected is the other direction of the same
// exemption: a shape-only block under any *other* name is a configuration error,
// not a container-relative one, and must keep failing exactly as it did.
func TestPartialNonSandboxRemoteStillRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  staging:
    capabilities:
      db: false
`)

	_, _, err := engine.LoadConfigFromDir(root, false)
	if err == nil || !strings.Contains(err.Error(), "remote 'staging' is missing host") {
		t.Fatalf("err = %v, want the shape-only staging block rejected", err)
	}
}

// TestMalformedNonSandboxRemoteFieldsStillRejected guards the rest of the
// per-remote loop the exemption skips: the synthetic name may not become a
// blanket pass for a block whose identity is genuinely broken.
func TestMalformedNonSandboxRemoteFieldsStillRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  staging:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
    auth:
      method: teleport
`)

	_, _, err := engine.LoadConfigFromDir(root, false)
	if err == nil || !strings.Contains(err.Error(), "unsupported auth method") {
		t.Fatalf("err = %v, want the unsupported auth method rejected", err)
	}
}

// TestSandboxBlockRejectsAnInvalidPort keeps the validator's exemption to what
// it claims to exempt. `sandbox` is exempt from *having* to name a target, not
// from the rule that a port it does name has to be a port: an out-of-range value
// is a claim about a machine that no reader may act on, and the desktop is
// exactly such a reader.
func TestSandboxBlockRejectsAnInvalidPort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  sandbox:
    port: 99999
`)

	_, _, err := engine.LoadConfigFromDir(root, false)
	if err == nil || !strings.Contains(err.Error(), "invalid port") {
		t.Fatalf("err = %v, want a remotes.sandbox block with port 99999 rejected", err)
	}
}

// TestSandboxBlockRejectsAnUnsupportedAuthMethod is the same boundary for the
// auth method: a block may omit it (the container owns it) but not name one
// govard does not support.
func TestSandboxBlockRejectsAnUnsupportedAuthMethod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  sandbox:
    auth:
      method: teleport
`)

	_, _, err := engine.LoadConfigFromDir(root, false)
	if err == nil || !strings.Contains(err.Error(), "unsupported auth method") {
		t.Fatalf("err = %v, want a remotes.sandbox block with auth.method teleport rejected", err)
	}
}

// TestSandboxBlockWithARedundantIdentityStillLoads is the other half: narrowing
// the exemption must not turn it back into a demand. A hand-written block that
// still names a valid port and a supported auth method stays loadable — those
// values are simply never read, which is a different thing from being invalid.
func TestSandboxBlockWithARedundantIdentityStillLoads(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  sandbox:
    port: 22
    auth:
      method: keyfile
    capabilities:
      db: false
`)

	cfg, _, err := engine.LoadConfigFromDir(root, false)
	if err != nil {
		t.Fatalf("a sandbox block with a valid port and auth method must still load: %v", err)
	}
	if _, ok := cfg.Remotes["sandbox"]; !ok {
		t.Fatal("remotes.sandbox did not survive validation")
	}
}

func TestRemoteListRendersConfiguredAndSandboxRows(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  staging:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "staging") {
		t.Fatalf("output missing the configured remote:\n%s", text)
	}
	if !strings.Contains(text, "sandbox") || !strings.Contains(text, "implicit") || !strings.Contains(text, "absent") {
		t.Fatalf("output missing the synthetic sandbox row:\n%s", text)
	}
}

func TestRemoteListDoesNotDuplicateSandboxRow(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  staging:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
  sandbox:
    host: legacy.example.com
    user: deploy
    path: /srv/legacy
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	rows := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(strings.ToLower(line), "sandbox") {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("expected exactly one sandbox row, got %d:\n%s", rows, out.String())
	}
}

// TestRemoteListPrintsOneSandboxRowForAPaddedKey is the padded-key half of the
// test above. `" sandbox "` is a key a hand-edit leaves behind, and it loads:
// IsValidRemoteName lowercases and trims before validating, and the config
// validator's sandbox exemption trims as well. Every other sandbox-name check in
// the tree trims too, so `remote list` was the only surface that did not — and a
// key that loads but is not recognised as the sandbox printed twice: as an
// ordinary remote with an empty host, the very shape the desktop's identity
// guard exists to keep off the panel, plus the sandbox's own row, with the
// "configured" note skipped because the block was never seen.
func TestRemoteListPrintsOneSandboxRowForAPaddedKey(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  " sandbox ":
    capabilities:
      db: false
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	rows := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(strings.ToLower(line), "sandbox") {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("a padded sandbox key must still be the sandbox, not a second row; got %d:\n%s", rows, out.String())
	}
	if !strings.Contains(errOut.String(), "layered over the synthetic sandbox") {
		t.Fatalf("a padded sandbox key is a configured sandbox and must be reported as such; stderr says: %q", errOut.String())
	}
}

// TestRemoteListReportsConfiguredSandboxOverSynthetic inverts
// TestShadowRemotesSandboxWarns. A configured `remotes.sandbox` block is no
// longer dead weight to be told to delete — it is load-bearing configuration, and
// the report has to say what it actually does: layer capabilities, protection
// and deploy settings over the container's identity.
func TestRemoteListReportsConfiguredSandboxOverSynthetic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  sandbox:
    host: legacy.example.com
    user: deploy
    path: /srv/legacy
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	if !strings.Contains(errOut.String(), "layered over the synthetic sandbox") {
		t.Fatalf("a configured remotes.sandbox must be reported as layered over the synthetic sandbox; stderr says: %q", errOut.String())
	}
	if strings.Contains(errOut.String(), "shadow") {
		t.Fatalf("the shadow warning is no longer true: stderr says: %q", errOut.String())
	}
}

// TestConfirmProtectedRemoteHonoursConfiguredSandboxProtection is the operator
// value of the whole feature: `protected` is the one field that is about intent
// rather than the container, so a project can require an explicit --yes for its
// own rehearsals even though the sandbox's identity is not theirs to set.
func TestConfirmProtectedRemoteHonoursConfiguredSandboxProtection(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return overlayBase(), deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	config := engine.Config{
		ProjectName: "sample-project",
		Remotes: engine.RemoteConfigMap{"sandbox": {
			Host:      "legacy.example.com",
			Protected: engine.BoolPtr(true),
		}},
	}
	execCmd := &cobra.Command{}
	execCmd.SetContext(context.Background())

	err := cmd.ConfirmProtectedRemoteForTest(execCmd, config, "sandbox", deploy.Options{}, "Deploy to")
	if err == nil {
		t.Fatal("err = <nil>, want the configured protection to gate the deploy")
	}
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *cli.UsageError: stdin is not a terminal, so there is nothing to confirm with", err, err)
	}
	if !strings.Contains(err.Error(), "protected") {
		t.Fatalf("err = %v, want a message naming the protection", err)
	}
}

// TestRemoteListShowsTheConfiguredSandboxCapabilities closes the gap between the
// note and the table: the note says the block's capabilities are layered, and
// the CAPABILITIES column — the only place an operator would look — carried the
// liveness word instead, so a configured restriction was invisible.
func TestRemoteListShowsTheConfiguredSandboxCapabilities(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  sandbox:
    capabilities:
      db: false
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf("no sandbox")
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}

	row := sandboxRowForTest(t, out.String())
	if !strings.Contains(row, "files") || !strings.Contains(row, "media") {
		t.Fatalf("sandbox row = %q, want the capabilities the block configures (db is blocked, so files+media remain)", row)
	}
	// The liveness is reported in the host column, in the same words
	// sandboxListState produces. Asserting the word itself is what makes this
	// line able to fail: a row that dropped the state entirely would leave a
	// substring check on the row name still passing.
	if !strings.Contains(row, "absent — govard sandbox up to create it") {
		t.Fatalf("sandbox row = %q, want the liveness in the host column", row)
	}
}

// TestRemoteListPrefersTheBlockCapabilitiesOverTheContainers is the guard the
// first implementation of that column lacked. It read the capabilities off the
// resolved remote and only fell back to the block when the container left them
// unset — which it does today, so the column was right by accident rather than
// by construction. The moment a container constrained (or granted) a capability,
// the column would have reported the container's value while cli-commands.md and
// deployment.md both promise it carries what the `remotes.sandbox` block
// configures.
//
// The stub below is the condition that never happens today: a running sandbox
// whose resolved remote carries capabilities of its own.
func TestRemoteListPrefersTheBlockCapabilitiesOverTheContainers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  sandbox:
    capabilities:
      db: false
`)
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{
			Host: "127.0.0.1", Port: 32998, User: deploy.SandboxUser,
			Auth: engine.RemoteAuth{Method: "keyfile"},
			Capabilities: &engine.RemoteCapabilities{
				Files: engine.BoolPtr(true),
				Media: engine.BoolPtr(true),
				DB:    engine.BoolPtr(true),
			},
		}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"remote", "list"})
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("remote list: %v", err)
	}

	// The row is `sandbox (implicit) <state> <capabilities> <auth> <key>`; the
	// state is one word here, so the capabilities column is the fourth field.
	fields := strings.Fields(sandboxRowForTest(t, out.String()))
	if len(fields) < 5 {
		t.Fatalf("sandbox row = %q, want the five columns", strings.Join(fields, " "))
	}
	if fields[2] != "running" {
		t.Fatalf("sandbox row state = %q, want the running sandbox reported in the host column", fields[2])
	}
	if fields[3] != "files,media" {
		t.Fatalf("sandbox capabilities column = %q, want the block's (db blocked), not the container's", fields[3])
	}
}

// sandboxRowForTest returns the single line of `remote list` output that names
// the synthetic remote.
func sandboxRowForTest(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), deploy.SandboxRemoteName) {
			return line
		}
	}
	t.Fatalf("no sandbox row in remote list output:\n%s", out)
	return ""
}

// TestRemoteAddReportsEveryIgnoredIdentityFlag is the user-visible half of the
// reversal, and the docs promise it unconditionally: every identity flag passed
// to `remote add sandbox` is dropped and named on stderr. `--strict-host-key`
// was the one registered flag the list missed, so passing it alone produced no
// output at all — a value discarded in silence on a command whose whole point
// is that your input was not used.
func TestRemoteAddReportsEveryIgnoredIdentityFlag(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: sample-project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	var errOut bytes.Buffer
	rootCmd := cmd.RootCommandForTest()
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{
		"remote", "add", "sandbox",
		"--host", "legacy.example.com",
		"--user", "intruder",
		"--port", "2222",
		"--path", "/srv/legacy",
		"--auth-method", "keyfile",
		"--key-path", "/tmp/other_key",
		"--known-hosts-file", "/tmp/known_hosts",
		"--strict-host-key",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("remote add sandbox: %v", err)
	}

	notice := errOut.String()
	for _, flag := range []string{"--host", "--user", "--port", "--path", "--auth-method", "--key-path", "--known-hosts-file", "--strict-host-key"} {
		if !strings.Contains(notice, flag) {
			t.Errorf("stderr does not name %s as ignored; the operator passed it and got no answer:\n%s", flag, notice)
		}
	}
}

// TestRemoteAddSandboxSuccessLineNamesTheContainerAuth is the other false
// signal the reversal introduced. The block stores no auth method — the write is
// skipped for a synthetic remote — so `NormalizeAuthMethod("")` reported
// `keychain` on a target whose real key is a keyfile the container provisioned.
// A SUCCESS line naming a property the target does not have is worse than no
// line at all.
func TestRemoteAddSandboxSuccessLineNamesTheContainerAuth(t *testing.T) {
	synthetic := cmd.RemoteAddSuccessLineForTest(deploy.SandboxRemoteName, engine.RemoteConfig{}, "keychain", false, true)
	if strings.Contains(synthetic, "keychain") {
		t.Errorf("sandbox confirmation = %q, want no auth method named: the container owns it", synthetic)
	}
	if !strings.Contains(synthetic, "container-derived") {
		t.Errorf("sandbox confirmation = %q, want the container-derived fact stated instead", synthetic)
	}

	ordinary := cmd.RemoteAddSuccessLineForTest("staging", engine.RemoteConfig{Auth: engine.RemoteAuth{StrictHostKey: true}}, "keyfile", false, false)
	if !strings.Contains(ordinary, "auth=keyfile") || !strings.Contains(ordinary, "strict_host_key=true") {
		t.Errorf("ordinary confirmation = %q, want the stored values reported unchanged", ordinary)
	}
}
