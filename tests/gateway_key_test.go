package tests

import (
	"os"
	"os/exec"
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
