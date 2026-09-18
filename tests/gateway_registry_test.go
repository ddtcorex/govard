package tests

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
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
	for _, user := range []string{"root", "nobody", "Root", "NOBODY"} {
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

func TestGatewayRegistryRevokeKeyRemovesAllMatches(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dataFor := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}
	for _, tc := range []struct{ material, comment string }{
		{"gateway-revoke-key-alpha", "shared-comment"},
		{"gateway-revoke-key-beta", "shared-comment"},
		{"gateway-revoke-key-gamma", "other-comment"},
	} {
		if _, err := reg.AllowKey("ssh-ed25519 " + dataFor(tc.material) + " " + tc.comment); err != nil {
			t.Fatalf("AllowKey: %v", err)
		}
	}
	if err := reg.RevokeKey("shared-comment"); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	if len(reg.Allowlist) != 1 {
		t.Fatalf("expected exactly one surviving key, got %d", len(reg.Allowlist))
	}
	if reg.Allowlist[0].Comment != "other-comment" {
		t.Fatalf("surviving key comment = %q, want %q", reg.Allowlist[0].Comment, "other-comment")
	}
}

func TestGatewayRegistryAllowKeyRejectsUnknownKeyType(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	keyData := base64.StdEncoding.EncodeToString([]byte("gateway-bad-type-key"))
	if _, err := reg.AllowKey("ssh-future " + keyData + " future@host"); err == nil {
		t.Fatal("expected an unknown key type to be rejected")
	} else if !strings.Contains(err.Error(), "ssh-future") {
		t.Fatalf("the error must name the key type, got: %v", err)
	}
	if len(reg.Allowlist) != 0 {
		t.Fatalf("expected empty allowlist after rejected key, got %d entries", len(reg.Allowlist))
	}
}

func TestGatewayRegistryKeysFileIsSortedByFingerprint(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dataA := base64.StdEncoding.EncodeToString([]byte("gateway-sort-key-alpha"))
	dataB := base64.StdEncoding.EncodeToString([]byte("gateway-sort-key-beta"))
	fpA, err := gateway.Fingerprint(dataA)
	if err != nil {
		t.Fatalf("Fingerprint A: %v", err)
	}
	fpB, err := gateway.Fingerprint(dataB)
	if err != nil {
		t.Fatalf("Fingerprint B: %v", err)
	}
	if fpA == fpB {
		t.Fatal("fixture keys collide; pick different material")
	}
	// Allow in reverse-sorted order, so insertion order alone is unsorted
	// and only an explicit sort produces the expected file order.
	first, second := dataA, dataB
	if fpA < fpB {
		first, second = dataB, dataA
	}
	if _, err := reg.AllowKey("ssh-ed25519 " + first + " sort-test"); err != nil {
		t.Fatalf("AllowKey first: %v", err)
	}
	if _, err := reg.AllowKey("ssh-ed25519 " + second + " sort-test"); err != nil {
		t.Fatalf("AllowKey second: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(gateway.GatewayDir(), "keys"))
	if err != nil {
		t.Fatalf("read keys file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 key lines, got %d:\n%s", len(lines), content)
	}
	got := []string{
		strings.SplitN(lines[0], " ", 2)[0],
		strings.SplitN(lines[1], " ", 2)[0],
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("keys file is not sorted by fingerprint:\n%s", content)
	}
}

func TestGatewayRenderFlatFilesCreatesMissingDir(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// No Save: RenderFlatFiles alone must create the gateway dir for
	// direct callers.
	if err := reg.RenderFlatFiles(); err != nil {
		t.Fatalf("RenderFlatFiles: %v", err)
	}
	for _, name := range []string{"targets", "keys"} {
		if _, err := os.Stat(filepath.Join(gateway.GatewayDir(), name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestGatewayRegistryRoundTripWithoutTargetsKey(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	if err := os.MkdirAll(gateway.GatewayDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A registry file written without the "targets" key (older writer,
	// hand edit) must round-trip to an explicit empty object, never null.
	raw := []byte("{\"allowlist\": []}\n")
	if err := os.WriteFile(filepath.Join(gateway.GatewayDir(), "registry.json"), raw, 0o600); err != nil {
		t.Fatalf("WriteFile registry: %v", err)
	}
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(gateway.GatewayDir(), "registry.json"))
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if !strings.Contains(string(content), "\"targets\": {}") {
		t.Fatalf("registry must round-trip a missing targets key to {}, got:\n%s", content)
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
