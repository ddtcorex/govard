package gateway

import (
	"strings"
	"testing"
)

// ssh-dss is a key type OpenSSH refuses by default since 7.0 and many builds
// omit entirely: allowing it at allow time reports success and then fails the
// login with a bare "permission denied".
func TestAllowKeyRejectsDSA(t *testing.T) {
	registry := &Registry{}
	_, err := registry.AllowKey("ssh-dss AAAAB3NzaC1kc3M= synthetic")
	if err == nil {
		t.Fatal("AllowKey accepted an ssh-dss key, want an error naming the type")
	}
	if !strings.Contains(err.Error(), "ssh-dss") {
		t.Fatalf("error %q does not name ssh-dss", err)
	}
}
