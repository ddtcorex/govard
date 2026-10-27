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

func TestResolveComposerVersionPrefersStackThenProfile(t *testing.T) {
	cases := []struct {
		name   string
		config engine.Config
		want   string
	}{
		{"stack pin wins", engine.Config{Framework: "magento2", FrameworkVersion: "2.4.6", Stack: engine.Stack{ComposerVersion: "2.7"}}, "2.7"},
		{"profile pin when stack is empty", engine.Config{Framework: "magento2", FrameworkVersion: "2.4.6"}, "2.2"},
		{"framework default when the line pins none", engine.Config{Framework: "magento2", FrameworkVersion: "2.4.9"}, "latest"},
		{"unknown framework", engine.Config{Framework: "no-such-framework"}, ""},
	}
	for _, tc := range cases {
		if got := engine.ResolveComposerVersion(tc.config); got != tc.want {
			t.Errorf("%s: ResolveComposerVersion() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The container's COMPOSER_HOME is created by Docker as root, because compose
// bind-mounts auth.json and config.json into a directory the image does not
// ship. A `docker exec -u root` runs with HOME=/root, so the historical repair
// (`chown -R www-data:www-data ~/.composer`) rewrote /root/.composer and left
// the real directory root-owned; the error was swallowed by `2>/dev/null`. This
// test pins the target to $COMPOSER_HOME by standing a fake chown in for the
// container's own.
func TestComposerHomeOwnershipScriptTargetsComposerHomeNotRootsHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script targets POSIX sh, not available on windows")
	}

	binDir := t.TempDir()
	recorded := filepath.Join(t.TempDir(), "chown-args")

	chownScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> '" + recorded + "'\n"
	if err := os.WriteFile(filepath.Join(binDir, "chown"), []byte(chownScript), 0o755); err != nil {
		t.Fatalf("write fake chown: %v", err)
	}

	root := t.TempDir()
	composerHome := filepath.Join(root, "home", "www-data", ".composer")
	rootHome := filepath.Join(root, "root")

	script := engine.ComposerHomeOwnershipScriptForTest()
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HOME="+rootHome,
		"COMPOSER_HOME="+composerHome,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ownership script failed: %v\noutput: %s", err, out)
	}

	raw, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatalf("fake chown was never called: %v", err)
	}
	args := strings.Fields(string(raw))
	if len(args) == 0 {
		t.Fatal("fake chown was called without arguments")
	}
	target := args[len(args)-1]
	if target != composerHome {
		t.Fatalf("chown target = %q, want the container COMPOSER_HOME %q (a root exec has HOME=%q)", target, composerHome, rootHome)
	}
	if strings.Contains(script, "~/") {
		t.Fatalf("ownership script must not rely on ~: a root exec resolves it to /root, script: %s", script)
	}
}
