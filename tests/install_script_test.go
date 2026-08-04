package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInstallScriptDesktopEligibility(t *testing.T) {
	availableAPTCache := installScriptAPTCache(t, 0)
	missingAPTCache := installScriptAPTCache(t, 1)
	webkitRuntime := installScriptWebKitRuntime(t)

	testCases := []struct {
		name    string
		pathDir string
		command string
	}{
		{
			name:    "available on Linux",
			pathDir: availableAPTCache,
			command: `OS=linux; CLI_ONLY=false; desktop_install_enabled`,
		},
		{
			name:    "falls back to CLI only when unavailable",
			pathDir: missingAPTCache,
			command: `OS=linux; CLI_ONLY=false; configure_desktop_install; test "$CLI_ONLY" = true`,
		},
		{
			name:    "available on macOS without APT",
			pathDir: t.TempDir(),
			command: `OS=darwin; CLI_ONLY=false; desktop_install_enabled`,
		},
		{
			name:    "available on Linux without APT when runtime is installed",
			pathDir: webkitRuntime,
			command: `PATH="$INSTALLER_TEST_PATH"; OS=linux; CLI_ONLY=false; desktop_install_enabled`,
		},
		{
			name:    "disabled explicitly",
			pathDir: availableAPTCache,
			command: `OS=linux; CLI_ONLY=true; if desktop_install_enabled; then exit 1; fi`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := runInstallScriptCommand(t, tc.pathDir, tc.command)
			if err != nil {
				t.Fatalf("installer policy command failed: %v\n%s", err, output)
			}
		})
	}
}

func TestInstallScriptHelpDocumentsCLIOnly(t *testing.T) {
	command := exec.Command("bash", "../install.sh", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("installer help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "--cli-only") {
		t.Fatalf("installer help does not document --cli-only:\n%s", output)
	}
}

func TestInstallationDocsDescribeCLIOnly(t *testing.T) {
	for _, path := range []string{
		"../README.md",
		"../docs/getting-started/installation.md",
		"../docs/vi/getting-started/installation.md",
	} {
		t.Run(path, func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read installation documentation: %v", err)
			}

			text := string(contents)
			for _, required := range []string{
				"--cli-only",
				"govard-desktop_<version>_linux_<arch>.deb",
			} {
				if !strings.Contains(text, required) {
					t.Errorf("documentation is missing %q", required)
				}
			}
			if !strings.Contains(text, "WebKitGTK 4.1") && !strings.Contains(text, "Ubuntu 22.04+") {
				t.Error("documentation does not state the Desktop platform requirement")
			}
		})
	}
}

func installScriptAPTCache(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "apt-cache")
	contents := "#!/usr/bin/env bash\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write apt-cache shim: %v", err)
	}

	// desktop_install_enabled() falls back to `sudo apt-get update` when APT
	// package lists are unpopulated (a real, machine-state-dependent
	// condition - see apt_lists_populated() in install.sh). sudo typically
	// enforces its own secure_path for the command it execs, ignoring our
	// PATH override, so shimming apt-get here would not reliably intercept
	// it. Shimming sudo itself does, keeping this test hermetic (no real
	// network access or privilege escalation) regardless of whether
	// /var/lib/apt/lists happens to be empty on the machine running the test.
	sudoPath := filepath.Join(dir, "sudo")
	if err := os.WriteFile(sudoPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write sudo shim: %v", err)
	}

	return dir
}

func installScriptWebKitRuntime(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ldconfig")
	contents := "#!/bin/sh\nprintf '%s\\n' 'libwebkit2gtk-4.1.so.0 (libc6,x86-64) => /usr/lib/libwebkit2gtk-4.1.so.0'\n"
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write ldconfig shim: %v", err)
	}
	return dir
}

func runInstallScriptCommand(t *testing.T, shimDir, command string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	script := "source \"$INSTALLER_PATH\"; " + command
	run := exec.CommandContext(ctx, "bash", "-c", script)
	run.Env = append(
		os.Environ(),
		"INSTALLER_PATH=../install.sh",
		"INSTALLER_TEST_PATH="+shimDir,
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := run.CombinedOutput()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, err
}
