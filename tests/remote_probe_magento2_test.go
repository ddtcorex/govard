package tests

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
	remote "govard/internal/engine/remote"
	"govard/internal/frameworks/magento2"
)

// Pins the served-candidate contract ProbeMagento2Environment must use: the
// configured path first, then public_html, then current.
func TestMagento2ProbeTriesServedCandidates(t *testing.T) {
	calls := []string{}
	probe := func(path string) (magento2.Magento2Environment, error) {
		calls = append(calls, path)
		return magento2.Magento2Environment{}, fmt.Errorf("%w at %s", remote.ErrProbeFilesNotFound, path)
	}
	_, _ = remote.TryProbeCandidatePaths("/var/www/app", probe)
	if len(calls) != 3 || calls[1] != "/var/www/app/public_html" || calls[2] != "/var/www/app/current" {
		t.Fatalf("expected 3 candidates, got %v", calls)
	}
}

// ProbeMagento2Environment must reach the served candidates when the
// configured path has no usable env.php. A fake ssh on PATH serves a valid
// payload only for the .../current candidate; the single-shot probe cannot
// succeed here.
func TestProbeMagento2EnvironmentFallsBackToServedCandidates(t *testing.T) {
	payload := encodeMagento2ProbePayload(t, map[string]string{
		"host":         "db.example:3307",
		"username":     "mage",
		"password":     "secret",
		"dbname":       "magento",
		"table_prefix": "demo_",
		"crypt_key":    "key123",
	})
	installFakeSSH(t, "/current", payload)

	env, err := magento2.ProbeMagento2Environment("fake", engine.RemoteConfig{
		Host: "fake.invalid",
		User: "deployer",
		Path: "/var/www/app",
	})
	if err != nil {
		t.Fatalf("ProbeMagento2Environment() error = %v", err)
	}
	if env.DB.Database != "magento" || env.DB.Username != "mage" {
		t.Fatalf("expected credentials from the served candidate, got %+v", env.DB)
	}
	if env.CryptKey != "key123" {
		t.Fatalf("expected crypt key key123, got %q", env.CryptKey)
	}
}

// A probe that reaches the target but finds no usable configuration must
// report ErrProbeFilesNotFound (via errors.Is) so the candidate loop can
// advance instead of aborting on the first miss.
func TestProbeMagento2EnvironmentMissWrapsFilesNotFound(t *testing.T) {
	installFakeSSH(t, "never-matches-any-candidate", "")

	_, err := magento2.ProbeMagento2Environment("fake", engine.RemoteConfig{
		Host: "fake.invalid",
		User: "deployer",
		Path: "/var/www/app",
	})
	if err == nil {
		t.Fatal("expected a miss when every candidate lacks env.php")
	}
	if !errors.Is(err, remote.ErrProbeFilesNotFound) {
		t.Fatalf("magento2 probe must wrap misses for candidate advance, got: %v", err)
	}
	for _, path := range []string{"/var/www/app", "/var/www/app/public_html", "/var/www/app/current"} {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("expected error to name %q, got: %v", path, err)
		}
	}
}

// A transport failure on the first candidate must abort the probe run
// directly instead of advancing: the fake ssh exits non-zero on the
// configured path, and the second candidate must never be attempted.
func TestProbeMagento2EnvironmentTransportAbortDoesNotAdvance(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not available for the fake ssh probe: %v", err)
	}
	dir := t.TempDir()
	advanced := filepath.Join(dir, "advanced")
	script := "#!/bin/sh\ncase \"$*\" in\n*public_html*|*/current*)\ntouch \"$FAKE_SSH_ADVANCED\"\n;;\n*)\necho fake transport failure >&2\nexit 1\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_SSH_ADVANCED", advanced)

	_, err := magento2.ProbeMagento2Environment("fake", engine.RemoteConfig{
		Host: "fake.invalid",
		User: "deployer",
		Path: "/var/www/app",
	})
	if err == nil {
		t.Fatal("expected transport error on the first candidate")
	}
	if errors.Is(err, remote.ErrProbeFilesNotFound) {
		t.Fatalf("transport error must abort instead of advancing as files-not-found, got: %v", err)
	}
	if !strings.Contains(err.Error(), "remote command failed") {
		t.Fatalf("expected the transport error directly, got: %v", err)
	}
	if _, statErr := os.Stat(advanced); !os.IsNotExist(statErr) {
		t.Fatalf("probe advanced to a later candidate after transport failure")
	}
}

func encodeMagento2ProbePayload(t *testing.T, payload map[string]string) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// installFakeSSH shadows the ssh binary with a script that prints payload
// only when the remote command mentions marker, and prints nothing otherwise
// (simulating a candidate without env.php). No network is touched.
func installFakeSSH(t *testing.T, marker string, payload string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not available for the fake ssh probe: %v", err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n*" + marker + "*)\nprintf '%s' \"$FAKE_SSH_PAYLOAD\"\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_SSH_PAYLOAD", payload)
}
