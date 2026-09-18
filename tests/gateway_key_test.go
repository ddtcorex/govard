package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/gateway"
)

func TestGatewaySecondHopKeyStable(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	p1, line1, err := gateway.EnsureSecondHopKey()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	p2, line2, err := gateway.EnsureSecondHopKey()
	if err != nil {
		t.Fatalf("re-ensure: %v", err)
	}
	if p1 != p2 || line1 != line2 {
		t.Fatalf("key not stable across calls: (%q,%q) vs (%q,%q)", p1, line1, p2, line2)
	}
	if !strings.HasPrefix(line1, "ssh-ed25519 ") {
		t.Fatalf("bad pub line %q", line1)
	}

	info, err := os.Stat(p1)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestGatewaySecondHopKeyRefusesDanglingPublicKey(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	dir := gateway.GatewayDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	privatePath := filepath.Join(dir, "id_ed25519")
	publicPath := filepath.Join(dir, "id_ed25519.pub")
	// Only the public half exists: the targets trust it, so regenerating
	// would lock the gateway out, and returning the path would hand out a
	// dangling identity. Fail loud instead.
	pubLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBog1oyN2RImNVwzmJhmgH5C+wr7v3d3g8W6xw4qzsvB govard-gateway"
	if err := os.WriteFile(publicPath, []byte(pubLine+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile public key: %v", err)
	}
	keyPath, _, err := gateway.EnsureSecondHopKey()
	if err == nil {
		t.Fatal("expected an error when the private half is missing")
	}
	if keyPath != "" {
		t.Fatalf("must not return a dangling path, got %q", keyPath)
	}
	if !strings.Contains(err.Error(), privatePath) {
		t.Fatalf("the error must mention the private path %q, got: %v", privatePath, err)
	}
}

func TestGatewaySecondHopKeyReuseEnforcesModes(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	keyPath, line1, err := gateway.EnsureSecondHopKey()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	publicPath := keyPath + ".pub"
	// Drift both modes: the reuse path must repair them without touching
	// the key material the targets already trust.
	if err := os.Chmod(keyPath, 0o640); err != nil {
		t.Fatalf("Chmod private: %v", err)
	}
	if err := os.Chmod(publicPath, 0o640); err != nil {
		t.Fatalf("Chmod public: %v", err)
	}
	keyPath2, line2, err := gateway.EnsureSecondHopKey()
	if err != nil {
		t.Fatalf("re-ensure: %v", err)
	}
	if keyPath2 != keyPath || line2 != line1 {
		t.Fatalf("reuse must keep the same key material: (%q,%q) vs (%q,%q)", keyPath, line1, keyPath2, line2)
	}
	if info, err := os.Stat(keyPath); err != nil {
		t.Fatalf("stat private key: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %v, want 0600", info.Mode().Perm())
	}
	if info, err := os.Stat(publicPath); err != nil {
		t.Fatalf("stat public key: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("public key mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestGatewaySecondHopKeyIsReadableByOpenSSH(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed; the live gateway integration test covers this")
	}
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	keyPath, publicLine, err := gateway.EnsureSecondHopKey()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	output, err := exec.Command(sshKeygen, "-y", "-f", keyPath).Output()
	if err != nil {
		t.Fatalf("ssh-keygen refused the generated key: %v", err)
	}
	parts := strings.SplitN(publicLine, " ", 3)
	expected := parts[0] + " " + parts[1]
	derived := strings.TrimSpace(string(output))
	if !strings.HasPrefix(derived, expected) {
		t.Fatalf("ssh-keygen derived\n%s\nwant a key starting with\n%s", derived, expected)
	}
}
