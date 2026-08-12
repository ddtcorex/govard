package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
)

// runComposerEnsureScript executes the generated script hermetically via the
// host's own /bin/sh, with fake `composer`/`curl` binaries standing in for
// the ones that would normally exist inside a project's PHP container.
func runComposerEnsureScript(t *testing.T, version, fakeInstalledVersion string) (output string, curlCalled bool) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("script targets POSIX sh, not available on windows")
	}

	binDir := t.TempDir()
	curlMarker := filepath.Join(t.TempDir(), "curl-called")

	composerScript := "#!/bin/sh\necho \"Composer version " + fakeInstalledVersion + " 2024-01-01 12:00:00\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "composer"), []byte(composerScript), 0o755); err != nil {
		t.Fatalf("write fake composer: %v", err)
	}

	curlScript := "#!/bin/sh\n" +
		"touch '" + curlMarker + "'\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-o\" ]; then\n" +
		"    shift\n" +
		"    echo fake-phar-content > \"$1\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(curlScript), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	downloadedPhar := filepath.Join(t.TempDir(), "composer.phar")
	script := engine.BuildComposerEnsureScriptForTest(version, "http://example.invalid/composer.phar")
	script = strings.ReplaceAll(script, "/tmp/composer.phar", downloadedPhar)

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\noutput: %s", err, out)
	}

	_, statErr := os.Stat(curlMarker)
	return string(out), statErr == nil
}

func TestComposerEnsureScriptSkipsDownloadWhenVersionAlreadyActive(t *testing.T) {
	output, curlCalled := runComposerEnsureScript(t, "2.9.8", "2.9.8")

	if curlCalled {
		t.Fatalf("expected curl NOT to run when the active Composer version already matches, output: %s", output)
	}
	if !strings.Contains(output, "already active, skipping download") {
		t.Fatalf("expected skip message in output, got: %s", output)
	}
}

func TestComposerEnsureScriptDownloadsWhenVersionDiffers(t *testing.T) {
	output, curlCalled := runComposerEnsureScript(t, "2.9.8", "2.8.0")

	if !curlCalled {
		t.Fatalf("expected curl to run when the active Composer version differs, output: %s", output)
	}
	if strings.Contains(output, "skipping download") {
		t.Fatalf("did not expect skip message, got: %s", output)
	}
}
