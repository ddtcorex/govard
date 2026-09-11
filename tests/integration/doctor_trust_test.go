//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDoctorJSONWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "doctor-deps-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	doctorResult := env.RunGovardWithEnv(t, projectDir, shim.Env(), "doctor", "--json")
	if doctorResult.ExitCode != 0 && doctorResult.ExitCode != 1 {
		t.Fatalf("expected doctor exit code 0 or 1, got %d\nstderr=%s", doctorResult.ExitCode, doctorResult.Stderr)
	}
	assertContains(t, doctorResult.Stdout, `"checks":`)
	// System dependencies are split by the command group they gate.
	assertContains(t, doctorResult.Stdout, `"host.deps.docker"`)
	assertContains(t, doctorResult.Stdout, `"host.deps.remote"`)
}

func TestTrustCommandWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "trust-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	installTrustDockerShim(t, shim)
	installRuntimeCommandShim(t, shim, "sudo", 0)

	homeDir := filepath.Join(projectDir, ".home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}

	result := env.RunGovardWithEnv(t, projectDir, append(shim.Env(), "HOME="+homeDir), "doctor", "trust")
	result.AssertSuccess(t)

	certPath := filepath.Join(homeDir, ".govard", "ssl", "root.crt")
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("expected extracted cert at %s: %v", certPath, err)
	}
	assertContains(t, string(certBytes), "BEGIN CERTIFICATE")

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|cp govard-proxy-caddy:/data/caddy/pki/authorities/local/root.crt "+certPath)
	switch runtime.GOOS {
	case "darwin":
		assertContains(t, logs, "sudo|security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "+certPath)
	case "linux":
		assertContains(t, logs, "sudo|cp "+certPath+" /usr/local/share/ca-certificates/govard.crt")
		assertContains(t, logs, "sudo|update-ca-certificates")
	default:
		t.Fatalf("unsupported trust command integration platform %q", runtime.GOOS)
	}
}

func installTrustDockerShim(t *testing.T, shims *RuntimeShims) {
	t.Helper()
	script := `#!/bin/sh
set -eu
log="${GOVARD_TEST_RUNTIME_LOG:-}"
if [ -n "$log" ]; then
  printf '%s|%s\n' "docker" "$*" >> "$log"
fi
if [ "$#" -ge 3 ] && [ "$1" = "cp" ]; then
  dest="$3"
  mkdir -p "$(dirname "$dest")"
  cat > "$dest" <<'EOF_CERT'
-----BEGIN CERTIFICATE-----
MIIDEzCCAfugAwIBAgIUciZkq4eXPE7ktpQ5jc8mUERKBe4wDQYJKoZIhvcNAQEL
BQAwGTEXMBUGA1UEAwwOR292YXJkIFRlc3QgQ0EwHhcNMjYwMzAzMDMxNTE5WhcN
MzYwMjI5MDMxNTE5WjAZMRcwFQYDVQQDDA5Hb3ZhcmQgVGVzdCBDQTCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBAKBpdvlGnEsieYi5mj/9dDPvT5Fkwbir
UvPmS/9ekFsAXNaqD6/XmM1vXHsFDf1P9OVPwnTkicq+iVShuekOMSzOI+ZOBG+C
GdZWnXUUny3wQBxAJLCcqqlp9aA1Y+XSn47TWPWmIAWNddxr0mvn2BloW4gDssss
g4egYlcbHHe7JxQZUEcHLm49uuE/o87y5KPtwdVi/B7pgmOh75+2N4XcxHp+rc0l
LHFZ+5QPZiW9N8Nl60N+1Wskx7wh7D/mvs7HUUEdFZ1f9WJQLAbEZr8kCROaNlUb
/58q7txwzvrb0pwlApkB6bi+gzOYGHPE3nRH6shTnSKexoas8aDlI2ECAwEAAaNT
MFEwHQYDVR0OBBYEFMSQLC8cLtJkqYX/sNhyHviQcyLsMB8GA1UdIwQYMBaAFMSQ
LC8cLtJkqYX/sNhyHviQcyLsMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggEBADP8g7Znris9/eFl8+Oclk/Zj9b0vkMTabZCj4wFxzbAmKA67OlwlnqU
IKA+pwBo1lNXpUWUQ06O/9eaNeuypWSqpFkNMqu91AD6Y6XWghxFBZ61bBIX3S1/
Kib0XGGkTTRkHjFyRcyhE9NmkSrM0MN9pvB5ACUvge8AEmWqG93paBVjuMTWgw1Z
6Gm/ewY5+pnMQvJEqyPAPVQS1kQ9UiL4SLi1EiM57/8Vot84u5lmaYn0jsZe0KTS
y8GOGWKnl7a+ZYmEoX841u9GcWl2EWAWmoIuE75YxmBDoUj5v8qD/LsJ2qOBckJu
tPqeILVmoltqkVAloHKAMzbHtwE7J2o=
-----END CERTIFICATE-----
EOF_CERT
fi
exit "${GOVARD_TEST_EXIT_DOCKER:-0}"
`
	path := filepath.Join(shims.Dir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to install docker trust shim: %v", err)
	}
}
