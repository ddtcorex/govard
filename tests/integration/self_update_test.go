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

// satisfiedNetCapability forces the `net` requirement for the tests below. They
// exercise `govard self-update` against local mock release servers, so letting
// the real connectivity probe decide would make the outcome depend on the
// host's network instead of on the code under test.
const satisfiedNetCapability = "GOVARD_TEST_SATISFIED_CAPABILITIES=net"

func TestSelfUpdateCommandDoesNotBlockInNonInteractiveMode(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "self-update-m2")

	result := runGovardWithTimeout(t, env, projectDir, 2*time.Second, []string{satisfiedNetCapability}, "self-update")
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
			satisfiedNetCapability,
			"GOVARD_SELF_UPDATE_CONFIRM=yes",
			"GOVARD_SELF_UPDATE_RELEASE_BASE_URL=" + mockReleaseServer.URL,
			"GOVARD_SKIP_DEP_CHECK=true",
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
		satisfiedNetCapability,
		"GOVARD_SELF_UPDATE_CONFIRM=yes",
		"GOVARD_SELF_UPDATE_RELEASE_BASE_URL="+mockReleaseServer.URL,
		"GOVARD_SKIP_DEP_CHECK=true",
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
	assertContains(t, output, "Updated govard-desktop at "+canonicalPathForTest(t, isolatedDesktopBinary))
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
