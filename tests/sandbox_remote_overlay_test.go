package tests

import (
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

// overlayBase mirrors what deploy.SandboxRemoteConfig produces for a running
// PHP sandbox: the container-derived identity, plus the four settings that
// describe the container itself.
func overlayBase() engine.RemoteConfig {
	return engine.RemoteConfig{
		Host: "127.0.0.1", Port: 32998, User: deploy.SandboxUser, Path: "/home/deployer/public_html",
		Sandbox: true, Protected: engine.BoolPtr(false),
		Auth: engine.RemoteAuth{Method: "keyfile", KeyPath: ".govard/sandbox/id_ed25519"},
		Deploy: &engine.DeployConfig{Repository: deploy.SandboxRepoPath, DeployPath: "/home/deployer/.deployer",
			Settings: map[string]any{"owner": "1000:1000", "writable_mode": "chmod_chown", "php_bin": "php", "php_version": "8.4"}},
	}
}

// TestSandboxRemoteOverlayKeepsContainerIdentity is the regression guard for the
// failure this whole package exists to prevent: a `remotes.sandbox` block that
// names another machine must still resolve to the container govard started. The
// block below is hostile on every identity field at once, so a leak shows up as
// a wrong host rather than as a subtle wrong-key comparison.
func TestSandboxRemoteOverlayKeepsContainerIdentity(t *testing.T) {
	hostile := engine.RemoteConfigMap{"sandbox": {
		Host: "legacy.example.com", User: "intruder", Port: 22, Path: "/wrong",
		URL: "https://legacy.example.com", Local: true, Sandbox: false,
		Auth: engine.RemoteAuth{Method: "keychain", KeyPath: "/tmp/other_key"},
	}}

	got := deploy.SandboxRemoteOverlay(overlayBase(), hostile)

	if got.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want the container-derived 127.0.0.1; a configured block must never repoint the rehearsal", got.Host)
	}
	if got.Port != 32998 {
		t.Errorf("Port = %d, want the container-published 32998", got.Port)
	}
	if got.User != deploy.SandboxUser {
		t.Errorf("User = %q, want %q", got.User, deploy.SandboxUser)
	}
	if got.Path != "/home/deployer/public_html" {
		t.Errorf("Path = %q, want the container's own document root", got.Path)
	}
	if got.URL != "" {
		t.Errorf("URL = %q, want empty: the synthetic sandbox publishes no site URL to configure", got.URL)
	}
	if got.Local {
		t.Error("Local = true, want false: local means 'run on this machine', which the container is not")
	}
	if !got.Sandbox {
		t.Error("Sandbox = false, want true: the flag is a topology fact about the remote, not a preference")
	}
	if got.Auth.Method != "keyfile" || got.Auth.KeyPath != ".govard/sandbox/id_ed25519" {
		t.Errorf("Auth = %+v, want the key `sandbox up` provisioned (keyfile, .govard/sandbox/id_ed25519)", got.Auth)
	}
}

// TestSandboxRemoteOverlayAppliesOnlyAllowedKeys pins the other half: what the
// block *may* change is exactly capabilities, protected and deploy settings,
// minus the four that describe the container itself.
func TestSandboxRemoteOverlayAppliesOnlyAllowedKeys(t *testing.T) {
	configured := engine.RemoteConfigMap{"sandbox": {
		Host:         "legacy.example.com",
		Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(false)},
		Protected:    engine.BoolPtr(true),
		Deploy: &engine.DeployConfig{KeepReleases: 7, Settings: map[string]any{
			"php_version": "8.9", "owner": "0:0", "writable_mode": "chmod", "php_bin": "/usr/bin/php8.9",
			"writable_permissions": "0777"}},
	}}

	got := deploy.SandboxRemoteOverlay(overlayBase(), configured)

	if got.Capabilities == nil || got.Capabilities.DB == nil || *got.Capabilities.DB {
		t.Errorf("Capabilities = %+v, want the configured db capability (db disabled)", got.Capabilities)
	}
	if got.Protected == nil || !*got.Protected {
		t.Errorf("Protected = %v, want the configured protection", got.Protected)
	}
	if got.Deploy == nil {
		t.Fatal("Deploy = nil, want the container deploy block with the configured settings layered on")
	}
	if got.Deploy.KeepReleases != 7 {
		t.Errorf("Deploy.KeepReleases = %d, want the configured 7", got.Deploy.KeepReleases)
	}
	if got.Deploy.Settings["writable_permissions"] != "0777" {
		t.Errorf("Deploy.Settings[writable_permissions] = %v, want the configured 0777 (not one of the pinned four)", got.Deploy.Settings["writable_permissions"])
	}
	for key, want := range map[string]any{
		"owner": "1000:1000", "writable_mode": "chmod_chown", "php_bin": "php", "php_version": "8.4",
	} {
		if got.Deploy.Settings[key] != want {
			t.Errorf("Deploy.Settings[%s] = %v, want the container-derived %v: the four pinned settings describe the container, not the rehearsal", key, got.Deploy.Settings[key], want)
		}
	}
	if got.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want the container-derived 127.0.0.1", got.Host)
	}
}

// TestSandboxRemoteOverlayDropsPinnedSettingsTheContainerLacks covers the
// inverse case: a sandbox image that ships no PHP must not acquire a
// php_version from configuration, or the deploy fails a PHP preflight for a
// reason that has nothing to do with the target machine.
func TestSandboxRemoteOverlayDropsPinnedSettingsTheContainerLacks(t *testing.T) {
	base := overlayBase()
	base.Deploy.Settings = map[string]any{"owner": "1000:1000"}
	configured := engine.RemoteConfigMap{"sandbox": {
		Deploy: &engine.DeployConfig{Settings: map[string]any{"php_version": "8.4", "php_bin": "php", "writable_permissions": "0777"}},
	}}

	got := deploy.SandboxRemoteOverlay(base, configured)

	if _, ok := got.Deploy.Settings["php_version"]; ok {
		t.Error("Deploy.Settings[php_version] was configured into a container that ships no PHP")
	}
	if _, ok := got.Deploy.Settings["php_bin"]; ok {
		t.Error("Deploy.Settings[php_bin] was configured into a container that ships no PHP")
	}
	if got.Deploy.Settings["writable_permissions"] != "0777" {
		t.Errorf("Deploy.Settings[writable_permissions] = %v, want the configured 0777", got.Deploy.Settings["writable_permissions"])
	}
}

// TestSandboxRemoteOverlayLeavesBaseAloneWithoutABlock is the no-op contract:
// a project that configures nothing gets byte-identical container identity, and
// a block under any other remote name is not this function's business.
func TestSandboxRemoteOverlayLeavesBaseAloneWithoutABlock(t *testing.T) {
	base := overlayBase()

	got := deploy.SandboxRemoteOverlay(base, nil)
	if got.Host != base.Host || got.Port != base.Port || got.User != base.User {
		t.Fatalf("overlay with no remotes changed the identity: %+v", got)
	}

	other := engine.RemoteConfigMap{"staging": {Host: "10.0.0.5", User: "deploy", Path: "/srv/staging"}}
	got = deploy.SandboxRemoteOverlay(base, other)
	if got.Host != base.Host || got.User != base.User {
		t.Fatalf("a remotes.staging block changed the sandbox identity: %+v", got)
	}
}

// TestSandboxRemoteOverlayMatchesTheBlockNameCaseInsensitively keeps the
// overlay aligned with `remote list` and ensureRemoteKnown, which both match the
// synthetic name case-insensitively; a block keyed "Sandbox" is the same block.
func TestSandboxRemoteOverlayMatchesTheBlockNameCaseInsensitively(t *testing.T) {
	configured := engine.RemoteConfigMap{"Sandbox": {
		Host:      "legacy.example.com",
		Protected: engine.BoolPtr(true),
		Deploy:    &engine.DeployConfig{KeepReleases: 7},
		Auth:      engine.RemoteAuth{Method: "keychain"},
		Path:      "/wrong",
		Local:     true,
	}}

	got := deploy.SandboxRemoteOverlay(overlayBase(), configured)

	if got.Protected == nil || !*got.Protected {
		t.Errorf("Protected = %v, want the configured protection; the block name matched case-insensitively but nothing was layered", got.Protected)
	}
	if got.Host != "127.0.0.1" || got.User != deploy.SandboxUser {
		t.Errorf("identity leaked from a case-insensitive block match: host=%q user=%q", got.Host, got.User)
	}
}

// TestSandboxRemoteOverlayPrefersTheExactBlockName removes the last source of
// run-to-run variation. Go randomises map iteration, so a project carrying both
// `sandbox:` and `Sandbox:` used to get an arbitrary one applied per invocation
// — the same deploy could rehearse against different settings on two runs. The
// exact spelling wins; the case-insensitive match is the fallback for a
// hand-edited block that is the only one present.
func TestSandboxRemoteOverlayPrefersTheExactBlockName(t *testing.T) {
	both := engine.RemoteConfigMap{
		"sandbox": {Deploy: &engine.DeployConfig{KeepReleases: 3}},
		"Sandbox": {Deploy: &engine.DeployConfig{KeepReleases: 9}},
	}

	// Repeated because the property is about an order that varies per process:
	// a single pass can agree by luck.
	for i := 0; i < 20; i++ {
		got := deploy.SandboxRemoteOverlay(overlayBase(), both)
		if got.Deploy == nil || got.Deploy.KeepReleases != 3 {
			t.Fatalf("iteration %d: KeepReleases = %v, want the exact `sandbox:` block's 3", i, got.Deploy.KeepReleases)
		}
	}
}
