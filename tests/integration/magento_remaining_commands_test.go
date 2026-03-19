//go:build integration
// +build integration

package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	govcmd "govard/internal/cmd"
)

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
	assertContains(t, logs, "sudo|cp "+certPath+" /usr/local/share/ca-certificates/govard.crt")
	assertContains(t, logs, "sudo|update-ca-certificates")
}

func TestDesktopCommandRuntimePaths(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "desktop-m2")

	t.Run("DesktopLaunchesBinaryFromPATH", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "govard-desktop", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "govard-desktop|")
	})

	t.Run("DesktopLaunchesBinaryWithBackgroundFlag", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "govard-desktop", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop", "--background")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "govard-desktop|--background")
	})

	t.Run("DesktopDevUsesWailsWhenAvailable", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "wails", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop", "--dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		if !strings.Contains(logs, "wails|dev -tags desktop") {
			t.Fatalf("expected 'wails|dev -tags desktop' in logs, got: %s\n\nstdout: %s\nstderr: %s", logs, result.Stdout, result.Stderr)
		}
	})
}

func TestSelfUpdateCommandDoesNotBlockInNonInteractiveMode(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "self-update-m2")

	result := runGovardWithTimeout(t, env, projectDir, 2*time.Second, nil, "self-update")
	if errorsContain(result.Error, "context deadline exceeded") {
		t.Fatalf("self-update blocked in non-interactive mode; output:\nstdout=%s\nstderr=%s", result.Stdout, result.Stderr)
	}
	result.AssertSuccess(t)
	assertContains(t, result.Stdout+result.Stderr, "Update cancelled.")
}

func TestSelfUpdateAutoConfirmViaEnv(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "self-update-confirm-m2")
	mockReleaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockReleaseServer.Close()

	result := runGovardWithTimeout(
		t,
		env,
		projectDir,
		10*time.Second,
		[]string{
			"GOVARD_SELF_UPDATE_CONFIRM=yes",
			"GOVARD_SELF_UPDATE_RELEASE_BASE_URL=" + mockReleaseServer.URL,
		},
		"self-update",
		"--version",
		"v1.0.2",
	)
	if errorsContain(result.Error, "context deadline exceeded") {
		t.Fatalf("self-update blocked even with GOVARD_SELF_UPDATE_CONFIRM=yes; output:\nstdout=%s\nstderr=%s", result.Stdout, result.Stderr)
	}
	output := result.Stdout + result.Stderr
	assertContains(t, output, "Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.")
	if strings.Contains(output, "Update cancelled.") {
		t.Fatalf("expected update flow to continue after auto-confirm; output:\n%s", output)
	}

	if result.Success() {
		t.Fatalf("expected self-update to fail against mock release server; output:\n%s", output)
	}
	assertContains(t, output, "failed with status 404")
}

func TestSelfUpdateAutoConfirmSuccessWithMockRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on Windows")
	}

	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "self-update-success-m2")
	releaseTag := "v1.0.2"

	archiveName, binaryName, err := govcmd.BuildReleaseAssetNameForTest("govard", releaseTag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("BuildReleaseAssetNameForTest failed: %v", err)
	}
	if !strings.HasSuffix(archiveName, ".tar.gz") {
		t.Skipf("unexpected archive format for integration test: %s", archiveName)
	}
	desktopArchiveName, desktopBinaryName, err := govcmd.BuildReleaseAssetNameForTest("govard-desktop", releaseTag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("BuildReleaseAssetNameForTest desktop failed: %v", err)
	}
	if !strings.HasSuffix(desktopArchiveName, ".tar.gz") {
		t.Skipf("unexpected desktop archive format for integration test: %s", desktopArchiveName)
	}

	archiveBody := buildTarGzBinaryAsset(t, binaryName, []byte("#!/bin/sh\necho govard\n"))
	desktopArchiveBody := buildTarGzBinaryAsset(t, desktopBinaryName, []byte("#!/bin/sh\necho govard-desktop\n"))
	checksum := sha256.Sum256(archiveBody)
	desktopChecksum := sha256.Sum256(desktopArchiveBody)
	checksumsBody := fmt.Sprintf(
		"%s  %s\n%s  %s\n",
		hex.EncodeToString(checksum[:]),
		archiveName,
		hex.EncodeToString(desktopChecksum[:]),
		desktopArchiveName,
	)

	mockReleaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(archiveBody)
		case "/" + desktopArchiveName:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(desktopArchiveBody)
		case "/checksums.txt":
			_, _ = w.Write([]byte(checksumsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockReleaseServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	isolatedDir := t.TempDir()
	isolatedBinary := filepath.Join(isolatedDir, "govard-self-update-test")
	copyBinaryForTest(t, env.BinaryPath, isolatedBinary)
	isolatedDesktopBinary := filepath.Join(isolatedDir, "govard-desktop")
	if err := os.WriteFile(isolatedDesktopBinary, []byte("stale-desktop"), 0o755); err != nil {
		t.Fatalf("failed to seed isolated desktop binary: %v", err)
	}

	cmd := exec.CommandContext(
		ctx,
		isolatedBinary,
		"self-update",
		"--version",
		releaseTag,
	)
	cmd.Dir = projectDir
	cmd.Env = envWithOverrides(
		os.Environ(),
		"GOVARD_SELF_UPDATE_CONFIRM=yes",
		"GOVARD_SELF_UPDATE_RELEASE_BASE_URL="+mockReleaseServer.URL,
		"PATH="+isolatedDir,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Error:  runErr,
	}
	if errorsContain(result.Error, "context deadline exceeded") {
		t.Fatalf("self-update blocked unexpectedly; output:\nstdout=%s\nstderr=%s", result.Stdout, result.Stderr)
	}
	output := result.Stdout + result.Stderr
	if !result.Success() {
		t.Fatalf("expected self-update to succeed against mock release server; output:\n%s", output)
	}
	desktopBytes, err := os.ReadFile(isolatedDesktopBinary)
	if err != nil {
		t.Fatalf("failed to read updated isolated desktop binary: %v", err)
	}
	if !strings.Contains(string(desktopBytes), "govard-desktop") {
		t.Fatalf("expected desktop binary to be replaced, got: %q", string(desktopBytes))
	}
	assertContains(t, output, "Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.")
	assertContains(t, output, "Checksum verified for "+archiveName+".")
	assertContains(t, output, "Checksum verified for "+desktopArchiveName+".")
	assertContains(t, output, "Updated govard-desktop at "+isolatedDesktopBinary)
	assertContains(t, output, "Successfully updated Govard to "+releaseTag)
}

func runGovardWithTimeout(t *testing.T, env *TestEnvironment, projectDir string, timeout time.Duration, extraEnv []string, args ...string) *CommandResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	isolatedBinary := filepath.Join(t.TempDir(), "govard-self-update-test")
	copyBinaryForTest(t, env.BinaryPath, isolatedBinary)

	cmd := exec.CommandContext(ctx, isolatedBinary, args...)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	return &CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
		Error:    err,
	}
}

func copyBinaryForTest(t *testing.T, src string, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read binary %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("failed to write isolated binary %s: %v", dst, err)
	}
}

func errorsContain(err error, needle string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), needle)
}

func envWithOverrides(base []string, overrides ...string) []string {
	out := []string{}
	overrideValues := map[string]string{}
	for _, pair := range overrides {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		overrideValues[parts[0]] = parts[1]
	}
	for _, pair := range base {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if _, exists := overrideValues[parts[0]]; exists {
			continue
		}
		out = append(out, pair)
	}
	for key, value := range overrideValues {
		out = append(out, key+"="+value)
	}
	return out
}

func buildTarGzBinaryAsset(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)

	header := &tar.Header{
		Name: binaryName,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return archive.Bytes()
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
