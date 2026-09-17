package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/gateway"
)

func TestGatewayRegistryAddTargetCollision(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.AddTarget("shop", gateway.Target{Container: "ctr-a", TargetUser: "deployer", Project: "shop"}); err != nil {
		t.Fatalf("AddTarget: %v", err)
	}
	err = reg.AddTarget("shop", gateway.Target{Container: "ctr-b", TargetUser: "deployer", Project: "other"})
	if err == nil || !strings.Contains(err.Error(), "ctr-a") {
		t.Fatalf("expected collision error naming ctr-a, got %v", err)
	}
}

func TestGatewayRouteUsername(t *testing.T) {
	for _, tc := range []struct {
		project string
		user    string
		ok      bool
	}{
		{"shop", "shop", true},
		{"my_shop", "my-shop", true},
		{"MyShop", "myshop", true},
		{"a b", "", false},
		{"", "", false},
	} {
		user, ok := gateway.RouteUsername(tc.project)
		if user != tc.user || ok != tc.ok {
			t.Errorf("RouteUsername(%q) = (%q, %v), want (%q, %v)", tc.project, user, ok, tc.user, tc.ok)
		}
	}
}

func TestGatewayRegistryRejectsReservedUsername(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, user := range []string{"root", "nobody"} {
		if err := reg.AddTarget(user, gateway.Target{Container: "ctr", TargetUser: "deployer", Project: "x"}); err == nil {
			t.Fatalf("expected the reserved username %s to be rejected", user)
		}
	}
}

func TestGatewayRegistryAllowKeyRejectsMultilineComment(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pubLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB a\nb"
	if _, err := reg.AllowKey(pubLine); err == nil {
		t.Fatal("expected a comment containing a newline to be rejected")
	}
	if len(reg.Allowlist) != 0 {
		t.Fatalf("expected empty allowlist after rejected key, got %d entries", len(reg.Allowlist))
	}
}

func TestGatewayRegistryRemoveTargetIsIdempotent(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Removing a user the registry never learned about (an old sandbox,
	// predating this feature) must not be an error.
	reg.RemoveTarget("never-registered")
}

func TestGatewayRegistryAllowKeyRendersKeyMaterial(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	pubLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB dev@laptop"
	fingerprint, err := reg.AllowKey(pubLine)
	if err != nil {
		t.Fatalf("AllowKey: %v", err)
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("fingerprint %q must start with SHA256:", fingerprint)
	}
	if strings.Contains(fingerprint, "=") {
		t.Fatalf("fingerprint %q must be unpadded base64 (no '='), like real ssh-keygen -lf output", fingerprint)
	}

	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(gateway.GatewayDir(), "keys"))
	if err != nil {
		t.Fatalf("stat keys file: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("keys file mode = %v, want 0644 (the sshd container's AuthorizedKeysCommandUser is not root)", info.Mode().Perm())
	}

	content, err := os.ReadFile(filepath.Join(gateway.GatewayDir(), "keys"))
	if err != nil {
		t.Fatalf("read keys file: %v", err)
	}
	if !strings.Contains(string(content), fingerprint) {
		t.Fatalf("keys file missing fingerprint %q:\n%s", fingerprint, content)
	}
	if !strings.Contains(string(content), "AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB") {
		t.Fatalf("keys file missing the raw key material authorized-keys-command.sh must re-emit:\n%s", content)
	}
	if !strings.Contains(string(content), "dev@laptop") {
		t.Fatalf("keys file missing the comment:\n%s", content)
	}
}

func TestGatewayRegistryAllowKeyIsIdempotentByFingerprint(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pubLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB dev@laptop"
	first, err := reg.AllowKey(pubLine)
	if err != nil {
		t.Fatalf("AllowKey: %v", err)
	}
	second, err := reg.AllowKey(pubLine)
	if err != nil {
		t.Fatalf("AllowKey again: %v", err)
	}
	if first != second {
		t.Fatalf("re-allowing the same key produced a different fingerprint: %q vs %q", first, second)
	}
	if len(reg.Allowlist) != 1 {
		t.Fatalf("expected exactly one allowlist entry, got %d", len(reg.Allowlist))
	}
}

func TestGatewayRegistryRevokeKeyRequiresAMatch(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.RevokeKey("SHA256:doesnotexist"); err == nil {
		t.Fatal("expected revoking an unknown fingerprint to error")
	}
}

func TestGatewayRegistrySaveEnforcesFileModes(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pubLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB dev@laptop"
	if _, err := reg.AllowKey(pubLine); err != nil {
		t.Fatalf("AllowKey: %v", err)
	}
	if err := reg.AddTarget("shop", gateway.Target{Container: "ctr-a", TargetUser: "deployer", Project: "shop"}); err != nil {
		t.Fatalf("AddTarget: %v", err)
	}
	// Pre-create the dir and keys file with wrong modes: Save must repair
	// them (WriteFile/MkdirAll alone would leave pre-existing modes intact).
	if err := os.MkdirAll(gateway.GatewayDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(gateway.GatewayDir(), 0o700); err != nil {
		t.Fatalf("Chmod dir: %v", err)
	}
	keysPath := filepath.Join(gateway.GatewayDir(), "keys")
	if err := os.WriteFile(keysPath, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("WriteFile keys: %v", err)
	}
	if err := os.Chmod(keysPath, 0o600); err != nil {
		t.Fatalf("Chmod keys: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if info, err := os.Stat(gateway.GatewayDir()); err != nil {
		t.Fatalf("stat gateway dir: %v", err)
	} else if info.Mode().Perm() != 0o755 {
		t.Fatalf("gateway dir mode = %v, want 0755", info.Mode().Perm())
	}
	for _, tc := range []struct {
		name string
		want os.FileMode
	}{
		{"registry.json", 0o600},
		{"targets", 0o644},
		{"keys", 0o644},
	} {
		info, err := os.Stat(filepath.Join(gateway.GatewayDir(), tc.name))
		if err != nil {
			t.Fatalf("stat %s: %v", tc.name, err)
		}
		if info.Mode().Perm() != tc.want {
			t.Fatalf("%s mode = %v, want %v", tc.name, info.Mode().Perm(), tc.want)
		}
	}
}
