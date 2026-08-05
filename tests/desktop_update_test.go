package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopCheckForUpdatesOutdated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	previousVersion := desktop.Version
	desktop.Version = "1.0.0"
	t.Cleanup(func() {
		desktop.Version = previousVersion
	})

	app := desktop.NewApp()
	result, err := app.CheckForUpdates()
	if err != nil {
		t.Fatalf("CheckForUpdates failed: %v", err)
	}
	if !result.Outdated {
		t.Fatalf("expected outdated=true, got false: %+v", result)
	}
	if result.CurrentVersion != "v1.0.0" {
		t.Fatalf("expected current version v1.0.0, got %q", result.CurrentVersion)
	}
	if result.LatestVersion != "v9.9.9" {
		t.Fatalf("expected latest version v9.9.9, got %q", result.LatestVersion)
	}
}

func TestDesktopCheckForUpdatesUpToDate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.1.1"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	previousVersion := desktop.Version
	desktop.Version = "1.1.1"
	t.Cleanup(func() {
		desktop.Version = previousVersion
	})

	app := desktop.NewApp()
	result, err := app.CheckForUpdates()
	if err != nil {
		t.Fatalf("CheckForUpdates failed: %v", err)
	}
	if result.Outdated {
		t.Fatalf("expected outdated=false, got true: %+v", result)
	}
	if !strings.Contains(result.Message, "up to date") {
		t.Fatalf("expected up-to-date message, got %q", result.Message)
	}
}

func TestDesktopInstallLatestUpdateRunsSelfUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("desktop self-update bridge is not supported on windows")
	}

	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	called := false
	restore := desktop.SetRunDesktopSelfUpdateForTest(func() (string, error) {
		called = true
		return "Successfully updated Govard to v1.16.1", nil
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.InstallLatestUpdate()
	if err != nil {
		t.Fatalf("InstallLatestUpdate failed: %v", err)
	}
	if !called {
		t.Fatal("expected self-update command to be called")
	}
	if !strings.Contains(message, "Successfully updated Govard") {
		t.Fatalf("unexpected install message: %q", message)
	}
}

func TestDesktopInstallLatestUpdateReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("desktop self-update bridge is not supported on windows")
	}

	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	restore := desktop.SetRunDesktopSelfUpdateForTest(func() (string, error) {
		return "", fmt.Errorf("boom")
	})
	defer restore()

	app := desktop.NewApp()
	_, err := app.InstallLatestUpdate()
	if err == nil {
		t.Fatal("expected InstallLatestUpdate to return an error")
	}
}

func TestDesktopSanitizeSelfUpdateOutputRemovesANSISequences(t *testing.T) {
	raw := "\u001b[30;46mINFO\u001b[0m Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.\n\u241B[30;46mINFO\u241B[0m Resolving latest release..."
	sanitized := desktop.SanitizeDesktopSelfUpdateOutputForTest(raw)

	if strings.Contains(sanitized, "\u001b") {
		t.Fatalf("expected sanitized output to remove ESC bytes, got %q", sanitized)
	}
	if strings.Contains(sanitized, "\u241B") {
		t.Fatalf("expected sanitized output to remove visible ESC symbol, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.") {
		t.Fatalf("expected sanitized output to keep text content, got %q", sanitized)
	}
}

func TestDesktopSummarizeSelfUpdateErrorPermissionDenied(t *testing.T) {
	sanitized := strings.Join([]string{
		"Govard Self-Update",
		"INFO Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.",
		"Error: permission denied replacing govard at /usr/local/bin/govard; re-run with elevated privileges: create temp target file: open /usr/local/bin/.govard-update-123: permission denied",
		"Usage:",
		"govard self-update [flags]",
	}, "\n")

	message := desktop.SummarizeDesktopSelfUpdateErrorForTest(fmt.Errorf("exit status 1"), sanitized)

	expected := `Update requires elevated privileges to modify /usr/local/bin/govard. Run "sudo govard self-update" in Terminal, then reopen Govard Desktop.`
	if message != expected {
		t.Fatalf("expected %q, got %q", expected, message)
	}
}

func TestDesktopSummarizeSelfUpdateErrorExtractsErrorLine(t *testing.T) {
	sanitized := strings.Join([]string{
		"Govard Self-Update",
		"INFO Resolving latest release...",
		"Error: failed to download checksums.txt",
		"Usage:",
		"govard self-update [flags]",
	}, "\n")

	message := desktop.SummarizeDesktopSelfUpdateErrorForTest(fmt.Errorf("exit status 1"), sanitized)
	if message != "failed to download checksums.txt" {
		t.Fatalf("expected extracted error message, got %q", message)
	}
}

func TestDesktopSummarizeSelfUpdateErrorAuthorizationDenied(t *testing.T) {
	sanitized := strings.Join([]string{
		"Error executing command as another user: Not authorized",
		"This incident has been reported.",
	}, "\n")

	message := desktop.SummarizeDesktopSelfUpdateErrorForTest(fmt.Errorf("exit status 126"), sanitized)
	expected := `Administrator authorization was not granted. Run "sudo govard self-update --yes" in Terminal, then reopen Govard Desktop.`
	if message != expected {
		t.Fatalf("expected %q, got %q", expected, message)
	}
}

func TestDesktopGetUpdateChannelDefaultsToStable(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	channel, err := app.GetUpdateChannel()
	if err != nil {
		t.Fatalf("GetUpdateChannel failed: %v", err)
	}
	if channel != "stable" {
		t.Fatalf("expected stable channel, got %q", channel)
	}
}

func TestDesktopSetUpdateChannelPersists(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	applied, err := app.SetUpdateChannel("beta")
	if err != nil {
		t.Fatalf("SetUpdateChannel failed: %v", err)
	}
	if applied != "beta" {
		t.Fatalf("expected applied channel beta, got %q", applied)
	}

	channel, err := app.GetUpdateChannel()
	if err != nil {
		t.Fatalf("GetUpdateChannel failed: %v", err)
	}
	if channel != "beta" {
		t.Fatalf("expected persisted channel beta, got %q", channel)
	}
}

func TestDesktopSetUpdateChannelRejectsInvalidValue(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	if _, err := app.SetUpdateChannel("nightly"); err == nil {
		t.Fatal("expected SetUpdateChannel to reject an invalid channel")
	}
}

func TestDesktopCheckForUpdatesUsesBetaChannelFeed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	if _, err := app.SetUpdateChannel("beta"); err != nil {
		t.Fatalf("SetUpdateChannel failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v9.9.9-beta.1"}]`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_LIST_URL", server.URL)
	previousVersion := desktop.Version
	desktop.Version = "1.0.0"
	t.Cleanup(func() {
		desktop.Version = previousVersion
	})

	result, err := app.CheckForUpdates()
	if err != nil {
		t.Fatalf("CheckForUpdates failed: %v", err)
	}
	if result.Channel != "beta" {
		t.Fatalf("expected result.Channel = beta, got %q", result.Channel)
	}
	if result.LatestVersion != "v9.9.9-beta.1" {
		t.Fatalf("expected latest version v9.9.9-beta.1, got %q", result.LatestVersion)
	}
}

func TestDesktopBuildElevatedSelfUpdateArgsForwardsGovardHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	args := desktop.BuildDesktopElevatedSelfUpdateArgsForTest("/usr/bin/env", "/usr/local/bin/govard", "")

	expected := fmt.Sprintf("GOVARD_HOME_DIR=%s", home)
	found := false
	for _, arg := range args {
		if arg == expected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected elevated pkexec args to include %q, got %v", expected, args)
	}
}

func TestDesktopBuildElevatedSelfUpdateArgsForwardsGovardHomeAlongsideDesktopTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	args := desktop.BuildDesktopElevatedSelfUpdateArgsForTest("/usr/bin/env", "/usr/local/bin/govard", "/opt/govard/govard-desktop")

	expectedHome := fmt.Sprintf("GOVARD_HOME_DIR=%s", home)
	expectedTarget := "GOVARD_SELF_UPDATE_DESKTOP_TARGET=/opt/govard/govard-desktop"
	foundHome := false
	foundTarget := false
	for _, arg := range args {
		if arg == expectedHome {
			foundHome = true
		}
		if arg == expectedTarget {
			foundTarget = true
		}
	}
	if !foundHome {
		t.Fatalf("expected elevated pkexec args to include %q, got %v", expectedHome, args)
	}
	if !foundTarget {
		t.Fatalf("expected elevated pkexec args to include %q, got %v", expectedTarget, args)
	}
}

func TestDesktopResolveGovardBinaryForUpdatePrefersSibling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	tmpDir := t.TempDir()
	runningDesktop := filepath.Join(tmpDir, "govard-desktop")
	siblingGovard := filepath.Join(tmpDir, "govard")
	pathGovard := filepath.Join(t.TempDir(), "path-govard")
	for _, candidate := range []string{runningDesktop, siblingGovard, pathGovard} {
		if err := os.WriteFile(candidate, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write fake binary %s: %v", candidate, err)
		}
	}

	restoreExec := desktop.SetDesktopExecutablePathForRestartForTest(func() (string, error) {
		return runningDesktop, nil
	})
	defer restoreExec()

	restoreLookPath := desktop.SetDesktopGovardLookPathForUpdateForTest(func(file string) (string, error) {
		return pathGovard, nil
	})
	defer restoreLookPath()

	resolved, err := desktop.ResolveGovardBinaryForDesktopUpdateForTest()
	if err != nil {
		t.Fatalf("resolve govard binary for desktop update failed: %v", err)
	}
	if !sameTestFile(t, resolved, siblingGovard) {
		t.Fatalf("expected sibling govard binary %q, got %q", siblingGovard, resolved)
	}
}

func TestDesktopResolveGovardBinaryForUpdateFallsBackToPATH(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	pathGovard := filepath.Join(t.TempDir(), "path-govard")
	if err := os.WriteFile(pathGovard, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake path govard: %v", err)
	}

	restoreExec := desktop.SetDesktopExecutablePathForRestartForTest(func() (string, error) {
		return "", fmt.Errorf("not running from binary")
	})
	defer restoreExec()

	restoreLookPath := desktop.SetDesktopGovardLookPathForUpdateForTest(func(file string) (string, error) {
		return pathGovard, nil
	})
	defer restoreLookPath()

	resolved, err := desktop.ResolveGovardBinaryForDesktopUpdateForTest()
	if err != nil {
		t.Fatalf("resolve govard binary for desktop update failed: %v", err)
	}
	if !sameTestFile(t, resolved, pathGovard) {
		t.Fatalf("expected PATH govard binary %q, got %q", pathGovard, resolved)
	}
}

func TestDesktopResolveDesktopBinaryForSelfUpdateTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	tmpDir := t.TempDir()
	runningDesktop := filepath.Join(tmpDir, "govard-desktop")
	if err := os.WriteFile(runningDesktop, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake desktop binary: %v", err)
	}

	restoreExec := desktop.SetDesktopExecutablePathForRestartForTest(func() (string, error) {
		return runningDesktop, nil
	})
	defer restoreExec()

	target := desktop.ResolveDesktopBinaryForSelfUpdateTargetForTest()
	if !sameTestFile(t, target, runningDesktop) {
		t.Fatalf("expected desktop target %q, got %q", runningDesktop, target)
	}
}

func TestDesktopRestartDesktopAppStartsBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "govard-desktop")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake desktop binary: %v", err)
	}

	restoreExec := desktop.SetDesktopExecutablePathForRestartForTest(func() (string, error) {
		return binaryPath, nil
	})
	defer restoreExec()

	restoreLookPath := desktop.SetDesktopBinaryLookPathForRestartForTest(func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	})
	defer restoreLookPath()

	called := ""
	restoreRestart := desktop.SetRestartDesktopBinaryForTest(func(path string) error {
		called = path
		return nil
	})
	defer restoreRestart()

	app := desktop.NewApp()
	message, err := app.RestartDesktopApp()
	if err != nil {
		t.Fatalf("RestartDesktopApp failed: %v", err)
	}
	if !strings.Contains(message, "Restarting Govard Desktop") {
		t.Fatalf("unexpected restart message: %q", message)
	}
	if !sameTestFile(t, called, binaryPath) {
		t.Fatalf("expected restart command to target %q, got %q", binaryPath, called)
	}
}

func TestDesktopRestartDesktopAppReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "govard-desktop")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake desktop binary: %v", err)
	}

	restoreExec := desktop.SetDesktopExecutablePathForRestartForTest(func() (string, error) {
		return binaryPath, nil
	})
	defer restoreExec()

	restoreRestart := desktop.SetRestartDesktopBinaryForTest(func(path string) error {
		return fmt.Errorf("boom")
	})
	defer restoreRestart()

	app := desktop.NewApp()
	_, err := app.RestartDesktopApp()
	if err == nil {
		t.Fatal("expected RestartDesktopApp to return an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to include boom, got %v", err)
	}
}
