package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/desktop"
	"govard/internal/engine"
)

// isolateOperationsLog points the operations log at a per-test temp file.
// `ListProjectRemotesByPathForTest` builds its last-sync labels from
// engine.ReadOperationEvents(5000), and that reader walks the log to EOF and
// JSON-parses every line before slicing the tail — so its cost scales with the
// total size of the log it finds, not with the limit. operationsLogPath()
// honours GOVARD_OPERATIONS_LOG_PATH but not GOVARD_HOME_DIR, so without this
// the four listing tests below read the developer's real ~/.govard/operations.log
// (40 MB / 200k lines on the machine this was measured on: 0.43s each, 3.4s each
// under -race against a 7s budget) and the cost would grow on a host with a
// longer history. It is the only thing those tests spend time on.
func isolateOperationsLog(t *testing.T) {
	t.Helper()
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))
}

// desktopSandboxProject writes a project whose only remote is the shape-only
// `remotes.sandbox` block `remote add sandbox` produces: no host, no user, no
// path, no auth. This is the state the reversal made reachable — before it a
// block like this could not even be loaded.
func desktopSandboxProject(t *testing.T) string {
	t.Helper()
	isolateOperationsLog(t)
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
    protected: true
`)
	return root
}

// desktopSandboxProjectWithoutBlock writes the same project with no
// `remotes.sandbox` block at all — the plain synthetic sandbox the resolver was
// designed for, and the one `govard remote list` always prints a row for.
func desktopSandboxProjectWithoutBlock(t *testing.T) string {
	t.Helper()
	isolateOperationsLog(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	return root
}

// stubDesktopSandbox mirrors production: deploy.ResolveSandboxRemoteForConfig
// resolves the container and then layers the project's `remotes.sandbox` block
// over it, and that overlay is where the block's capabilities reach the caller.
// A stub returning the bare container config made every assertion about a
// layered value vacuous.
func stubDesktopSandbox(remote engine.RemoteConfig, liveness deploy.SandboxLiveness, err error) func() {
	return desktop.StubResolveSandboxRemoteForDesktopForTest(
		func(ctx context.Context, cfg engine.Config, projectRoot string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
			if err != nil {
				return engine.RemoteConfig{}, liveness, err
			}
			return deploy.SandboxRemoteOverlay(remote, cfg.Remotes), liveness, nil
		},
	)
}

// TestDesktopListsTheSandboxFromTheContainer is the desktop half of the
// reversal. The block is loadable, so the desktop used to list it as a remote
// and resolve it from the map — an entry with an empty host, a defaulted port of
// 22 and the operator's own `localhost` as its target. It must list what the
// container actually is, with the block's capabilities layered on.
func TestDesktopListsTheSandboxFromTheContainer(t *testing.T) {
	root := desktopSandboxProject(t)
	defer stubDesktopSandbox(overlayBase(), deploy.SandboxLivenessRunning, nil)()

	snapshot, err := desktop.ListProjectRemotesByPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}
	if len(snapshot.Remotes) != 1 {
		t.Fatalf("remotes = %+v, want exactly the sandbox row", snapshot.Remotes)
	}
	row := snapshot.Remotes[0]
	if row.Name != deploy.SandboxRemoteName {
		t.Fatalf("row name = %q, want the sandbox", row.Name)
	}
	if row.Host != "127.0.0.1" || row.Port != 32998 || row.User != deploy.SandboxUser {
		t.Errorf("row identity = %s:%d@%s, want the container's 127.0.0.1:32998@%s",
			row.Host, row.Port, row.User, deploy.SandboxUser)
	}
	if row.AuthMethod != "keyfile" {
		t.Errorf("row auth = %q, want the container's keyfile", row.AuthMethod)
	}
	// The fixture blocks `db` in the `remotes.sandbox` block, so a row that
	// carried the block's capabilities cannot offer a database. It is the only
	// assertion here that can tell a layered remote from a bare container one.
	for _, capability := range row.Capabilities {
		if capability == engine.RemoteCapabilityDB {
			t.Errorf("row capabilities = %v, want the block's: db is blocked by remotes.sandbox", row.Capabilities)
		}
	}
	if len(row.Capabilities) == 0 {
		t.Errorf("row capabilities = %v, want the capabilities the block leaves open", row.Capabilities)
	}
	if len(snapshot.Warnings) != 0 {
		t.Errorf("warnings = %v, want none: the sandbox resolved", snapshot.Warnings)
	}
}

// TestDesktopResolvesTheSandboxThroughTheContainer pins the resolve path the
// admin/SFTP/SSH actions use. It must be whatever the deploy resolver answers —
// the container identity with the block layered over it — and never the raw
// block, which is what used to hand the URL builders an empty host.
func TestDesktopResolvesTheSandboxThroughTheContainer(t *testing.T) {
	container := overlayBase()
	container.Port = 41337
	var seenCfg engine.Config
	var seenRoot string
	restore := desktop.StubResolveSandboxRemoteForDesktopForTest(
		func(ctx context.Context, cfg engine.Config, projectRoot string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
			seenCfg = cfg
			seenRoot = projectRoot
			return container, deploy.SandboxLivenessRunning, nil
		})
	defer restore()

	cfg := engine.Config{
		ProjectName: "sample-project",
		Framework:   "generic",
		Remotes: engine.RemoteConfigMap{deploy.SandboxRemoteName: {
			Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)},
			Protected:    engine.BoolPtr(true),
		}},
	}
	root := t.TempDir()

	name, resolved, err := desktop.ResolveRemoteConfigForCapabilityForTest(cfg, "sandbox", engine.RemoteCapabilityFiles, root)
	if err != nil {
		t.Fatalf("resolve sandbox: %v", err)
	}
	if name != deploy.SandboxRemoteName {
		t.Fatalf("name = %q, want %q", name, deploy.SandboxRemoteName)
	}
	if resolved.Host != "127.0.0.1" || resolved.Port != 41337 {
		t.Fatalf("resolved = %s:%d, want the container's identity, not the block's empty host", resolved.Host, resolved.Port)
	}
	if seenRoot != root {
		t.Errorf("resolver got project root %q, want %q: the desktop knows which project it resolved and must say so", seenRoot, root)
	}
	if _, ok := seenCfg.Remotes[deploy.SandboxRemoteName]; !ok {
		t.Error("resolver got a config without the remotes.sandbox block, so the block could not be layered")
	}
}

// TestDesktopRefusesTheSandboxWhenTheContainerIsGone is the failure the raw
// reader turned into a wrong destination: with no container there is nothing to
// be a remote, and the desktop must say so instead of falling back to the
// operator's own machine.
func TestDesktopRefusesTheSandboxWhenTheContainerIsGone(t *testing.T) {
	restore := stubDesktopSandbox(engine.RemoteConfig{}, deploy.SandboxLivenessAbsent,
		fmt.Errorf("unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it"))
	defer restore()

	cfg := engine.Config{
		ProjectName: "sample-project",
		Framework:   "generic",
		Remotes:     engine.RemoteConfigMap{deploy.SandboxRemoteName: {}},
	}

	_, _, err := desktop.ResolveRemoteConfigForCapabilityForTest(cfg, "sandbox", engine.RemoteCapabilityFiles, t.TempDir())
	if err == nil {
		t.Fatal("resolving sandbox with no container succeeded, want the resolver's message")
	}
	if !strings.Contains(err.Error(), "no sandbox exists for this project") {
		t.Fatalf("err = %v, want the synthetic resolver's own message", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "localhost") {
		t.Fatalf("err = %v, want no localhost target named", err)
	}
}

// TestDesktopDropsAnUnresolvableSandboxRow pairs with the refusal above: a
// listing that rendered the block itself would put an empty host back into the
// UI, and the first click on it would be the localhost fallback again. The row
// goes; the reason comes back as a warning.
func TestDesktopDropsAnUnresolvableSandboxRow(t *testing.T) {
	root := desktopSandboxProject(t)
	defer stubDesktopSandbox(engine.RemoteConfig{}, deploy.SandboxLivenessDormant,
		fmt.Errorf("sandbox container govard-sample-sandbox-abc123 is not running; run 'govard sandbox up' to start it"))()

	snapshot, err := desktop.ListProjectRemotesByPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}
	for _, row := range snapshot.Remotes {
		if strings.EqualFold(row.Name, deploy.SandboxRemoteName) {
			t.Fatalf("sandbox row %+v was listed although the container is not running", row)
		}
	}
	if len(snapshot.Warnings) != 1 || !strings.Contains(snapshot.Warnings[0], "is not running") {
		t.Fatalf("warnings = %v, want one naming the resolver's reason", snapshot.Warnings)
	}
}

// TestDesktopRefusesARemoteWithNoHost closes the fallback itself: the desktop's
// URL builders turn an empty host into "localhost", which would point the
// operator's own machine at their admin URL and report the remote as opened. A
// remote that resolves to no host at all is refused before any URL is built.
func TestDesktopRefusesARemoteWithNoHost(t *testing.T) {
	cfg := engine.Config{
		ProjectName: "sample-project",
		Remotes: engine.RemoteConfigMap{
			"staging": {Capabilities: &engine.RemoteCapabilities{}},
		},
	}

	_, _, err := desktop.ResolveRemoteConfigForCapabilityForTest(cfg, "staging", "", t.TempDir())
	if err == nil {
		t.Fatal("a remote with no host resolved, want the localhost substitution to be refused")
	}
	if !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("err = %v, want a message naming what is being refused", err)
	}
}

// TestDesktopListsTheSandboxWithoutABlock closes the gap between the two
// surfaces. The listing resolved the synthetic name only for projects that wrote
// a `remotes.sandbox` block, so a project using the plain synthetic sandbox — the
// configuration it was designed for, and the one `govard remote list` always
// prints a row for — had no sandbox row at all, and its Open admin / Open SFTP /
// Open SSH buttons were unreachable. The row's presence must depend on the
// container answering, not on how the project is configured.
func TestDesktopListsTheSandboxWithoutABlock(t *testing.T) {
	root := desktopSandboxProjectWithoutBlock(t)
	defer stubDesktopSandbox(overlayBase(), deploy.SandboxLivenessRunning, nil)()

	snapshot, err := desktop.ListProjectRemotesByPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}
	if len(snapshot.Remotes) != 1 {
		t.Fatalf("remotes = %+v, want the sandbox row `remote list` also prints", snapshot.Remotes)
	}
	row := snapshot.Remotes[0]
	if row.Name != deploy.SandboxRemoteName {
		t.Fatalf("row name = %q, want the sandbox", row.Name)
	}
	if row.Host != "127.0.0.1" || row.Port != 32998 {
		t.Fatalf("row identity = %s:%d, want the container's, not a configured block's", row.Host, row.Port)
	}
	if len(snapshot.Warnings) != 0 {
		t.Errorf("warnings = %v, want none: the sandbox resolved", snapshot.Warnings)
	}
}

// TestDesktopDoesNotWarnAboutASandboxNobodyConfigured is the other direction of
// the change above, and the reason the warning is conditional. Resolving the
// synthetic name for every project means every project without a container gets
// a resolver error; a warning for a remote the project never asked for would put
// a permanent red line in a panel that is otherwise clean. `remote list` says
// nothing in the same case either — its `absent` word is the row, not a warning.
func TestDesktopDoesNotWarnAboutASandboxNobodyConfigured(t *testing.T) {
	root := desktopSandboxProjectWithoutBlock(t)
	defer stubDesktopSandbox(engine.RemoteConfig{}, deploy.SandboxLivenessAbsent,
		fmt.Errorf("unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it"))()

	snapshot, err := desktop.ListProjectRemotesByPathForTest(root)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}
	if len(snapshot.Remotes) != 0 {
		t.Fatalf("remotes = %+v, want none: there is no container to list", snapshot.Remotes)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none: the project configured no remotes.sandbox block", snapshot.Warnings)
	}
}

// TestDesktopSandboxRequestIsNotServedFromTheBlock proves the branch is keyed on
// the name, not on the block's presence: a project with no `remotes.sandbox` at
// all still resolves `sandbox` from the container, which is the whole contract
// of the synthetic remote.
func TestDesktopSandboxRequestIsNotServedFromTheBlock(t *testing.T) {
	restore := stubDesktopSandbox(overlayBase(), deploy.SandboxLivenessRunning, nil)
	defer restore()

	cfg := engine.Config{ProjectName: "sample-project", Framework: "generic"}

	name, resolved, err := desktop.ResolveRemoteConfigForCapabilityForTest(cfg, "Sandbox", engine.RemoteCapabilityFiles, t.TempDir())
	if err != nil {
		t.Fatalf("resolve Sandbox: %v", err)
	}
	if name != deploy.SandboxRemoteName {
		t.Fatalf("name = %q, want %q: the synthetic name is case-insensitive everywhere else", name, deploy.SandboxRemoteName)
	}
	if resolved.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want the container's", resolved.Host)
	}
}
